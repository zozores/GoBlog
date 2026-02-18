package main

import (
	"bytes"
	"os"
	"testing"

	ts "git.jlel.se/jlelse/template-strings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderMarkdownAlerts(t *testing.T) {
	// Initialize TemplateStrings using local FS
	tstrings, err := ts.InitTemplateStringsFS(os.DirFS("."), "strings", ".yaml", "default")
	require.NoError(t, err)

	app := &goBlog{
		ts:  tstrings,
		cfg: &config{},
	}

	t.Run("Note Alert Default Lang", func(t *testing.T) {
		markdown := `> [!NOTE]
> This is a note.`
		var buf bytes.Buffer
		err := app.renderMarkdownToWriter(&buf, markdown, false, "")
		require.NoError(t, err)
		html := buf.String()
		assert.Contains(t, html, "markdown-alert-note")
		assert.Contains(t, html, "Note")
	})

	t.Run("Warning Alert", func(t *testing.T) {
		markdown := `> [!WARNING]
> Watch out!`
		var buf bytes.Buffer
		err := app.renderMarkdownToWriter(&buf, markdown, false, "")
		require.NoError(t, err)
		html := buf.String()
		assert.Contains(t, html, "markdown-alert-warning")
		assert.Contains(t, html, "Warning")
	})

	t.Run("Important Alert", func(t *testing.T) {
		markdown := `> [!IMPORTANT]
> This is important!`
		var buf bytes.Buffer
		err := app.renderMarkdownToWriter(&buf, markdown, false, "")
		require.NoError(t, err)
		html := buf.String()
		assert.Contains(t, html, "markdown-alert-important")
		assert.Contains(t, html, "Important")
	})

	t.Run("Multiple Alerts", func(t *testing.T) {
		markdown := `> [!NOTE]
> Note 1

> [!TIP]
> Tip 2

> [!WARNING]
> Warning 3`
		var buf bytes.Buffer
		err := app.renderMarkdownToWriter(&buf, markdown, false, "")
		require.NoError(t, err)
		html := buf.String()
		assert.Contains(t, html, "markdown-alert-note")
		assert.Contains(t, html, "Note 1")
		assert.Contains(t, html, "markdown-alert-tip")
		assert.Contains(t, html, "Tip 2")
		assert.Contains(t, html, "markdown-alert-warning")
		assert.Contains(t, html, "Warning 3")
	})
}
