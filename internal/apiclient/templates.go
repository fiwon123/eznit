package apiclient

import (
	"net/http"
	"net/url"

	"github.com/fiwon123/eznit/internal/domain/templates"
	"github.com/fiwon123/eznit/pkg/types"
)

// Templates: the catalogue and one template's contents. The DTOs are the
// domain package's rather than a second copy of them, so a field the server
// grows is a field this client decodes without another definition to keep in
// step — the same choice the CLI makes.

// ListTemplates returns the catalogue: every public template, plus the
// caller's own private ones when the client carries a session.
//
// The kind filter is a query parameter on the catalogue route; the "mine"
// route takes none, so a caller that wants both applies FilterTemplates to the
// result.
func (c *Client) ListTemplates(mine bool, kind string) ([]templates.TemplateData, error) {
	path := "/v1/templates"
	if mine {
		path += "/mine"
	} else if kind != "" {
		path += "?" + url.Values{"kind": {kind}}.Encode()
	}

	var envelope types.Envelope[templates.ListResponse]

	if err := c.do(http.MethodGet, path, nil, &envelope); err != nil {
		return nil, err
	}

	if envelope.Data.Data == nil {
		return []templates.TemplateData{}, nil
	}

	return envelope.Data.Data, nil
}

// GetTemplate returns one template with its files, variables and commands in
// the same answer: a text file carries its content and a binary file carries
// base64, so the detail is the whole template.
func (c *Client) GetTemplate(id string) (*templates.TemplateDetail, error) {
	var envelope types.Envelope[templates.TemplateDetail]

	if err := c.do(http.MethodGet, "/v1/templates/"+url.PathEscape(id), nil, &envelope); err != nil {
		return nil, err
	}

	return &envelope.Data, nil
}

// FilterTemplates keeps the templates of one kind; an empty kind keeps them
// all. The "mine" route cannot filter on the server, so a caller asking for
// both filters here.
func FilterTemplates(items []templates.TemplateData, kind string) []templates.TemplateData {
	if kind == "" {
		return items
	}

	kept := make([]templates.TemplateData, 0, len(items))

	for _, item := range items {
		if item.Kind == kind {
			kept = append(kept, item)
		}
	}

	return kept
}
