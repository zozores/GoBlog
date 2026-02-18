package main

import (
	"io"
	"strings"

	marktag "git.jlel.se/jlelse/goldmark-mark"
	"github.com/yuin/goldmark"
	emoji "github.com/yuin/goldmark-emoji"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"go.goblog.app/app/pkgs/builderpool"
	"go.goblog.app/app/pkgs/highlighting"
	"go.goblog.app/app/pkgs/htmlbuilder"
)

func (a *goBlog) initMarkdown() {
	a.initMarkdownOnce.Do(func() {
		defaultGoldmarkOptions := []goldmark.Option{
			goldmark.WithRendererOptions(
				html.WithUnsafe(),
			),
			goldmark.WithParserOptions(
				parser.WithAutoHeadingID(),
			),
			goldmark.WithExtensions(
				extension.Table,
				extension.Strikethrough,
				extension.Footnote,
				extension.Typographer,
				extension.Linkify,
				marktag.Mark,
				emoji.Emoji,
				highlighting.Highlighting,
				&alertExtension{app: a},
			),
		}
		publicAddress := ""
		if srv := a.cfg.Server; srv != nil {
			publicAddress = srv.PublicAddress
		}
		a.md = goldmark.New(append(defaultGoldmarkOptions, goldmark.WithExtensions(&customExtension{
			absoluteLinks: false,
			publicAddress: publicAddress,
		}))...)
		a.absoluteMd = goldmark.New(append(defaultGoldmarkOptions, goldmark.WithExtensions(&customExtension{
			absoluteLinks: true,
			publicAddress: publicAddress,
		}))...)
		a.titleMd = goldmark.New(
			goldmark.WithParser(
				// Override, no need for special Markdown parsers
				parser.NewParser(
					parser.WithBlockParsers(util.Prioritized(parser.NewParagraphParser(), 1000)),
				),
			),
			goldmark.WithExtensions(
				extension.Typographer,
				emoji.Emoji,
			),
		)
	})
}

var contextKeyLang = parser.NewContextKey()

func (a *goBlog) renderMarkdownToWriter(w io.Writer, source string, absoluteLinks bool, lang string) (err error) {
	a.initMarkdown()
	ctx := parser.NewContext()
	ctx.Set(contextKeyLang, lang)
	if absoluteLinks {
		err = a.absoluteMd.Convert([]byte(source), w, parser.WithContext(ctx))
	} else {
		err = a.md.Convert([]byte(source), w, parser.WithContext(ctx))
	}
	return err
}

func (a *goBlog) renderText(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	pr, pw := io.Pipe()
	go func() {
		_ = pw.CloseWithError(a.renderMarkdownToWriter(pw, s, false, ""))
	}()
	text, err := htmlTextFromReader(pr)
	_ = pr.CloseWithError(err)
	if err != nil {
		return "", nil
	}
	return text, nil
}

func (a *goBlog) renderTextSafe(s string) string {
	r, _ := a.renderText(s)
	return r
}

func (a *goBlog) renderMdTitle(s string) string {
	if s == "" {
		return ""
	}
	a.initMarkdown()
	pr, pw := io.Pipe()
	go func() {
		_ = pw.CloseWithError(a.titleMd.Convert([]byte(s), pw))
	}()
	text, err := htmlTextFromReader(pr)
	_ = pr.CloseWithError(err)
	if err != nil {
		return ""
	}
	return text
}

// Extensions etc...

// Links
type customExtension struct {
	publicAddress string
	absoluteLinks bool
}

func (l *customExtension) Extend(m goldmark.Markdown) {
	m.Renderer().AddOptions(renderer.WithNodeRenderers(
		util.Prioritized(&customRenderer{
			absoluteLinks: l.absoluteLinks,
			publicAddress: l.publicAddress,
		}, 500),
	))
}

type customRenderer struct {
	publicAddress string
	absoluteLinks bool
}

func (c *customRenderer) RegisterFuncs(r renderer.NodeRendererFuncRegisterer) {
	r.Register(ast.KindLink, c.renderLink)
	r.Register(ast.KindImage, c.renderImage)
}

func (c *customRenderer) renderLink(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	hb := htmlbuilder.NewHtmlBuilder(w)
	if entering {
		n := node.(*ast.Link)
		dest := string(n.Destination)
		if c.absoluteLinks && c.publicAddress != "" {
			resolved, err := resolveURLReferences(c.publicAddress, dest)
			if err != nil {
				return ast.WalkStop, err
			}
			if len(resolved) > 0 {
				dest = resolved[0]
			}
		}
		tagOpts := []any{"href", dest}
		if isAbsoluteURL(string(n.Destination)) {
			tagOpts = append(tagOpts, "target", "_blank", "rel", "noopener")
		}
		if n.Title != nil {
			tagOpts = append(tagOpts, "title", string(n.Title))
		}
		hb.WriteElementOpen("a", tagOpts...)
	} else {
		hb.WriteElementClose("a")
	}
	return ast.WalkContinue, nil
}

func (c *customRenderer) renderImage(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.Image)
	dest := string(n.Destination)
	// Make destination absolute if it's relative
	if c.absoluteLinks && c.publicAddress != "" {
		resolved, err := resolveURLReferences(c.publicAddress, dest)
		if err != nil {
			return ast.WalkStop, err
		}
		if len(resolved) > 0 {
			dest = resolved[0]
		}
	}
	hb := htmlbuilder.NewHtmlBuilder(w)
	hb.WriteElementOpen("a", "href", dest)
	imgEls := []any{"src", dest, "alt", c.extractTextFromChildren(n, source), "loading", "lazy"}
	if len(n.Title) > 0 {
		imgEls = append(imgEls, "title", string(n.Title))
	}
	hb.WriteElementOpen("img", imgEls...)
	hb.WriteElementClose("a")
	return ast.WalkSkipChildren, nil
}

func (r *customRenderer) extractTextFromChildren(node ast.Node, source []byte) string {
	if node == nil {
		return ""
	}
	b := builderpool.Get()
	defer builderpool.Put(b)
	for c := node.FirstChild(); c != nil; c = c.NextSibling() {
		if s, ok := c.(*ast.String); ok {
			b.Write(s.Value)
		} else if t, ok := c.(*ast.Text); ok {
			b.Write(t.Segment.Value(source))
		} else {
			b.WriteString(r.extractTextFromChildren(c, source))
		}
	}
	return b.String()
}

// Alerts

type alertExtension struct {
	app *goBlog
}

func (e *alertExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(
		parser.WithASTTransformers(
			util.Prioritized(&alertASTTransformer{}, 500),
		),
	)
	m.Renderer().AddOptions(
		renderer.WithNodeRenderers(
			util.Prioritized(&alertHTMLRenderer{app: e.app}, 500),
		),
	)
}

type alertASTTransformer struct{}

func (a *alertASTTransformer) Transform(node *ast.Document, reader text.Reader, pc parser.Context) {
	lang, _ := pc.Get(contextKeyLang).(string)

	var alerts []*ast.Blockquote

	ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		bq, ok := n.(*ast.Blockquote)
		if !ok {
			return ast.WalkContinue, nil
		}
		if bq.ChildCount() == 0 {
			return ast.WalkContinue, nil
		}
		firstParams, ok := bq.FirstChild().(*ast.Paragraph)
		if !ok {
			return ast.WalkContinue, nil
		}
		content := string(firstParams.Lines().Value(reader.Source()))

		if !strings.HasPrefix(content, "[!") {
			return ast.WalkContinue, nil
		}

		alerts = append(alerts, bq)
		return ast.WalkSkipChildren, nil
	})

	for _, bq := range alerts {
		firstParams := bq.FirstChild().(*ast.Paragraph)
		content := string(firstParams.Lines().Value(reader.Source()))

		closeBracket := strings.Index(content, "]")
		if closeBracket == -1 {
			continue
		}

		typeStr := strings.TrimSpace(content[2:closeBracket])
		alertType := ""

		switch strings.ToUpper(typeStr) {
		case "NOTE":
			alertType = "note"
		case "TIP":
			alertType = "tip"
		case "IMPORTANT":
			alertType = "important"
		case "WARNING":
			alertType = "warning"
		case "CAUTION":
			alertType = "caution"
		}

		if alertType == "" {
			continue
		}

		// Check that the rest of the line is empty/whitespace by checking bounds
		// content usually includes the newline at end of the line segment
		rest := content[closeBracket+1:]
		if idx := strings.IndexAny(rest, "\r\n"); idx != -1 {
			rest = rest[:idx]
		}
		if strings.TrimSpace(rest) != "" {
			continue
		}

		// Create Alert node
		alert := &Alert{
			BaseBlock: ast.BaseBlock{},
			AlertType: alertType,
			Lang:      lang,
		}

		// Logic to remove the [!TYPE] line
		// We use the segment of the first line to determine the cutoff point in source
		firstLineSeg := firstParams.Lines().At(0)
		cutoff := firstLineSeg.Stop

		// Remove text nodes belonging to the first line
		var next ast.Node
		for curr := firstParams.FirstChild(); curr != nil; curr = next {
			next = curr.NextSibling()

			if tNode, ok := curr.(*ast.Text); ok {
				nodeSeg := tNode.Segment
				// Check for overlap with first line
				if nodeSeg.Stop <= cutoff {
					// Fully inside first line -> Remove
					firstParams.RemoveChild(firstParams, curr)
				} else if nodeSeg.Start < cutoff {
					// Overlaps -> Trim
					tNode.Segment = text.NewSegment(cutoff, nodeSeg.Stop)
					// We trimmed the overlap, so the rest belongs to next line.
					// Stop processing.
					break
				} else {
					// Strictly after -> Stop
					break
				}
			} else {
				// Non-text node found.
				// Since we enforced strict syntax, the first line should only contain text.
				// If we encounter a non-text node, it must be on the next line or invalid.
				// We stop.
				break
			}
		}

		if firstParams.ChildCount() == 0 {
			bq.RemoveChild(bq, firstParams)
		}

		// Now move all children
		parent := bq.Parent()
		if parent == nil {
			continue
		}

		// Move children
		curr := bq.FirstChild()
		for curr != nil {
			next := curr.NextSibling()
			bq.RemoveChild(bq, curr)
			alert.AppendChild(alert, curr)
			curr = next
		}

		parent.ReplaceChild(parent, bq, alert)
	}
}

type Alert struct {
	ast.BaseBlock
	AlertType string
	Lang      string
}

func (n *Alert) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

var KindAlert = ast.NewNodeKind("Alert")

func (n *Alert) Kind() ast.NodeKind {
	return KindAlert
}

type alertHTMLRenderer struct {
	app *goBlog
}

func (r *alertHTMLRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(KindAlert, r.renderAlert)
}

func (r *alertHTMLRenderer) renderAlert(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*Alert)
	if entering {
		w.WriteString("<div class=\"markdown-alert markdown-alert-")
		w.WriteString(n.AlertType)
		w.WriteString("\">")
		w.WriteString("<p class=\"markdown-alert-title\">")
		// Localize title
		title := r.app.ts.GetTemplateStringVariant(n.Lang, "alert"+n.AlertType)
		if title == "" {
			// Fallback
			title = strings.Title(n.AlertType)
		}
		w.WriteString(title)
		w.WriteString("</p>")
	} else {
		w.WriteString("</div>")
	}
	return ast.WalkContinue, nil
}
