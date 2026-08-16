package server

import (
	_ "embed"
	"html/template"
	"net/http"
	"sync"

	tgju "github.com/amiranmanesh/tgju-api-go"
)

//go:embed openapi.yaml
var openAPISpec []byte

//go:embed docs.html
var docsTemplateSource string

// docsTemplate is parsed once, on first use, so a malformed template shows up
// in the tests rather than at init time in someone's production binary.
var docsTemplate = sync.OnceValue(func() *template.Template {
	return template.Must(template.New("docs").Parse(docsTemplateSource))
})

func (s *Server) handleOpenAPI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write(openAPISpec)
}

// handleDocs renders a self-contained reference page.
//
// It embeds no CDN script and no external stylesheet on purpose: this binary is
// meant to run inside a container on a network that may not reach the open
// internet, and documentation that only works online is documentation that
// fails when you need it.
func (s *Server) handleDocs(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Version  string
		Upstream string
		Routes   []Route
	}{
		Version:  tgju.Version,
		Upstream: s.client.BaseURL(),
		Routes:   s.Routes(),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")

	if err := docsTemplate().Execute(w, data); err != nil {
		loggerFrom(r).ErrorContext(r.Context(), "server: could not render the documentation page")
	}
}
