package tfdocgen

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"text/template"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

type Provider struct {
	Name              string
	TerraformProvider *schema.Provider
	DisplayNameFunc   func(name string) string
	CategoryNameFunc  func(name string) string
	CategoriesFunc    func() []string
}

func (p *Provider) init() {
	if p.DisplayNameFunc == nil {
		p.DisplayNameFunc = func(name string) string {
			return name
		}
	}
	if p.CategoryNameFunc == nil {
		p.CategoryNameFunc = func(name string) string {
			return name
		}
	}
	if p.CategoriesFunc == nil {
		p.CategoriesFunc = func() []string {
			params, err := p.Parameters()
			if err != nil {
				return []string{}
			}
			nameMap := make(map[string]struct{})
			for _, p := range params {
				nameMap[p.SubCategory] = struct{}{}
			}
			var names []string
			for key := range nameMap {
				names = append(names, key)
			}
			return names
		}
	}
}

func (p *Provider) Parameters() ([]*TemplateParameter, error) {
	p.init()

	var parameters []*TemplateParameter

	// Provider
	parameters = append(parameters, &TemplateParameter{
		Type:                TypeProvider,
		ProviderName:        p.Name,
		ProviderDisplayName: p.DisplayNameFunc(p.Name),
		Name:                p.Name,
		DisplayName:         p.DisplayNameFunc(p.Name),
		Schema:              NewSchema(p.TerraformProvider.Schema),
	})

	// Resources
	for _, rt := range p.TerraformProvider.Resources() {
		rs := p.TerraformProvider.ResourcesMap[rt.Name]
		parameters = append(parameters, &TemplateParameter{
			Type:                TypeResource,
			ProviderName:        p.Name,
			ProviderDisplayName: p.DisplayNameFunc(p.Name),
			Name:                rt.Name,
			DisplayName:         p.DisplayNameFunc(rt.Name),
			SubCategory:         p.CategoryNameFunc(rt.Name),
			Schema:              NewSchema(rs.Schema),
			IsImportable:        rt.Importable,
			Timeouts:            rs.Timeouts,
		})
	}

	// DataSources
	for _, dt := range p.TerraformProvider.DataSources() {
		ds := p.TerraformProvider.DataSourcesMap[dt.Name]
		parameters = append(parameters, &TemplateParameter{
			Type:                TypeDataSource,
			ProviderName:        p.Name,
			ProviderDisplayName: p.DisplayNameFunc(p.Name),
			Name:                dt.Name,
			DisplayName:         p.DisplayNameFunc(dt.Name),
			SubCategory:         p.CategoryNameFunc(dt.Name),
			Schema:              NewSchema(ds.Schema),
		})
	}
	return parameters, nil
}

func (p *Provider) GenerateDocs(templateDir, exampleDir, destDir string) error {
	parameters, err := p.Parameters()
	if err != nil {
		return err
	}

	// generate erb
	if err := p.writeERBFile(destDir, parameters); err != nil {
		return err
	}

	for _, param := range parameters {
		tmpl, err := p.loadTemplate(templateDir, param.TemplatePath())
		if err != nil {
			return err
		}
		example, err := p.loadExample(exampleDir, param.ExamplePath())
		if err != nil {
			return err
		}
		param.Example = example

		dest := filepath.Join(destDir, param.Destination())
		if err := p.execTemplate(tmpl, dest, param); err != nil {
			return err
		}
	}
	return nil
}

func (p *Provider) writeERBFile(destDir string, params []*TemplateParameter) error {
	erbParams := p.buildERBParameters(params)

	dest := filepath.Join(destDir, p.Name+".erb")
	if err := p.prepareDestDir(dest); err != nil {
		return err
	}

	buf := bytes.NewBufferString("")
	t := template.New("t")
	template.Must(t.Parse(erbTemplate))
	if err := t.Execute(buf, erbParams); err != nil {
		return err
	}

	// write to file
	if err := ioutil.WriteFile(dest, buf.Bytes(), 0644); err != nil {
		return err
	}
	fmt.Println(dest)
	return nil
}

func (p *Provider) buildERBParameters(params []*TemplateParameter) []*erbParameter {
	var results []*erbParameter
	categories := p.CategoriesFunc()
	for _, category := range categories {
		rs, ds := p.extractByCategory(category, params)
		results = append(results, &erbParameter{
			CategoryName: category,
			DataSources:  ds,
			Resources:    rs,
		})
	}
	return results
}

func (p *Provider) extractByCategory(category string, params []*TemplateParameter) (resources []*TemplateParameter, dataSources []*TemplateParameter) {
	for _, p := range params {
		if p.SubCategory != category {
			continue
		}
		switch p.Type {
		case TypeResource:
			resources = append(resources, p)
		case TypeDataSource:
			dataSources = append(dataSources, p)
		}
	}
	return resources, dataSources
}

func (p *Provider) execTemplate(tmpl, dest string, param *TemplateParameter) error {
	if err := p.prepareDestDir(dest); err != nil {
		return err
	}

	buf := bytes.NewBufferString("")
	t := template.New("t")
	template.Must(t.Parse(tmpl))
	if err := t.Execute(buf, param); err != nil {
		return err
	}

	// write to file
	if err := ioutil.WriteFile(dest, buf.Bytes(), 0644); err != nil {
		return err
	}
	fmt.Println(dest)
	return nil
}

func (p *Provider) loadTemplate(templateDir, templatePath string) (string, error) {
	tmplPath := filepath.Join(templateDir, templatePath)
	if _, err := os.Stat(tmplPath); os.IsNotExist(err) {
		return defaultTemplate, nil
	}

	data, err := ioutil.ReadFile(tmplPath)
	if err != nil {
		return "", err
	}
	return string(data), err
}

func (p *Provider) loadExample(exampleDir, examplePath string) (string, error) {
	exPath := filepath.Join(exampleDir, examplePath)

	if _, err := os.Stat(exPath); os.IsNotExist(err) {
		return "", nil
	}

	data, err := ioutil.ReadFile(exPath)
	if err != nil {
		return "", err
	}
	return string(data), err
}

func (p *Provider) prepareDestDir(destPath string) error {
	dir := filepath.Dir(destPath)
	_, err := os.Stat(dir)
	if err != nil {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}

type erbParameter struct {
	CategoryName string
	DataSources  []*TemplateParameter
	Resources    []*TemplateParameter
}

const erbTemplate = `<% wrap_layout :inner do %>
  <% content_for :sidebar do %>
    <div class="docs-sidebar hidden-print affix-top" role="complementary">
      <a href="#" class="subnav-toggle">(Expand/collapse all)</a>
      <ul class="nav docs-sidenav">
        <li>
          <a href="/docs/providers/index.html">All Providers</a>
        </li>

        <li>
          <a href="/docs/providers/gmailfilter/index.html">GmailFilter Provider</a>
        </li>

{{ range . }}{{ if or .DataSources .Resources }}
        <li>
          <a href="#">{{ .CategoryName }}</a>
          <ul class="nav">
{{ if .DataSources }}
            <li>
              <a href="#">Data Sources</a>
              <ul class="nav nav-auto-expand">
{{- range .DataSources }}
                <li>
                  <a href="{{ .Link }}">{{ .Name }}</a>
                </li>
{{- end }}
              </ul>
            </li>
{{ end }}
{{ if .Resources }}
            <li>
              <a href="#">Resources</a>
              <ul class="nav nav-auto-expand">
{{- range .Resources }}
                <li>
                  <a href="{{ .Link }}">{{ .Name }}</a>
                </li>
{{- end }}
              </ul>
            </li>
{{ end }}
          </ul>
        </li>
{{ end }}{{ end }}
      </ul>
    </div>
  <% end %>

  <%= yield %>
<% end %>
`

const defaultTemplate = `---
layout: "{{ .Layout }}"
page_title: "{{ .PageTitle }}"
{{ if .SubCategory -}}
subcategory: "{{ .SubCategory}}"
{{ end -}}
description: |-
  {{ .ShortDescription }}
---

# {{ .Title }}

{{ .Description }}

{{ if .Example -}}
## Example Usage

` + "```" + `hcl
{{ .Example}}
` + "```" + `
{{ end -}}


{{ if .Schema.Arguments -}}
## Argument Reference

{{ range .Schema.Arguments -}}
* ` + "`" + `{{ .Name }}` + "`" + ` - ({{ .RequiredOrOptional }}) {{ .Description }}.{{if .ForceNew }} Changing this forces a new resource to be created.{{ end }}{{ if .Default }} Default:{{ .DefaultString }}.{{ end }}
{{ end }}

{{ range .Schema.ArgumentBlocks -}}
---

A ` + "`" + `{{ .Name }}` + "`" + ` block supports the following:

{{ range .Arguments -}}
* ` + "`" + `{{ .Name }}` + "`" + ` - ({{ .RequiredOrOptional }}) {{ .Description }}.
{{ end }}
{{ end -}}
{{ if .HasTimeouts }}
### Timeouts

The ` + "`" + `timeouts` + "`" + ` block allows you to specify [timeouts](https://www.terraform.io/docs/configuration/resources.html#operation-timeouts) for certain actions:

{{ if .TimeoutsCreate }}* ` + "`" + `create` + "`" + ` - (Defaults to {{ .TimeoutsCreate }}) Used when creating the {{ .DisplayName }}
{{ end }}
{{ if .TimeoutsRead   }}* ` + "`" + `read` + "`" + ` -   (Defaults to {{ .TimeoutsRead   }}) Used when reading the {{ .DisplayName }}
{{ end }}
{{ if .TimeoutsUpdate }}* ` + "`" + `update` + "`" + ` - (Defaults to {{ .TimeoutsUpdate }}) Used when updating the {{ .DisplayName }}
{{ end }}
{{ if .TimeoutsDelete }}* ` + "`" + `delete` + "`" + ` - (Defaults to {{ .TimeoutsDelete }}) Used when deleting {{ .DisplayName }}
{{ end }}

{{ end }}
{{ end -}}

{{ if not .IsProvider -}}
## Attribute Reference

* ` + "`" + `id` + "`" + ` - The id of the {{ .DisplayName }}.
{{ range .Schema.Attributes -}}
* ` + "`" + `{{ .Name }}` + "`" + ` - {{ .Description }}.
{{ end }}

{{ range .Schema.AttributeBlocks -}}
---

A ` + "`" + `{{ .Name }}` + "`" + ` block exports the following:

{{ range .Attributes -}}
* ` + "`" + `{{ .Name }}` + "`" + ` - {{ .Description }}.
{{ end }}
{{ end }}
{{ end -}}
`
