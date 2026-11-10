package hml

import (
	"fmt"
	"strings"
	"testing"
)

// benchRow is a struct so the loop exercises evalField's reflection path
// (NumField scan + struct-tag parse) rather than the map fast path.
type benchRow struct {
	Name string `json:"name"`
	Cls  string `json:"cls"`
	ID   int64  `json:"id"`
}

// BenchmarkRenderLoopStructs mirrors BenchmarkRenderLoop but iterates over a
// []benchRow, so every field access goes through evalField's struct reflection.
func BenchmarkRenderLoopStructs(b *testing.B) {
	src := "%ul\n" +
		"  - for row in rows\n" +
		"    %li{ class: row.cls, \"data-id\": row.id }\n" +
		"      = row.name\n"
	tmpl, err := Parse(src, "bench.hml", nil)
	if err != nil {
		b.Fatal(err)
	}
	rows := make([]benchRow, 100)
	for i := range rows {
		rows[i] = benchRow{Name: fmt.Sprintf("Row %d", i), Cls: "row", ID: int64(i)}
	}
	locals := map[string]any{"rows": rows}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := tmpl.Render(locals, nil); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRenderLoopInterp exercises string-literal interpolation inside
// expressions (an interpolated attribute value), the path evalInterpString
// serves. Pre-parsing the #{} expression at tokenize time removes a
// per-render, per-iteration re-parse here.
func BenchmarkRenderLoopInterp(b *testing.B) {
	src := "%ul\n" +
		"  - for row in rows\n" +
		"    %a{ href: \"/companies/show?id=#{row.id}\", class: \"c-#{row.cls}\" }\n" +
		"      = row.name\n"
	tmpl, err := Parse(src, "bench.hml", nil)
	if err != nil {
		b.Fatal(err)
	}
	rows := make([]benchRow, 100)
	for i := range rows {
		rows[i] = benchRow{Name: fmt.Sprintf("Row %d", i), Cls: "row", ID: int64(i)}
	}
	locals := map[string]any{"rows": rows}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := tmpl.Render(locals, nil); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRenderPartialsLoop exercises the partial path: a partial rendered
// per loop item under a wide (~50-key) layout map, inherited through the
// layered context, mimicking how www renders partials.
func BenchmarkRenderPartialsLoop(b *testing.B) {
	rowSrc := "%li{ class: cls, \"data-id\": id }\n" +
		"  = name\n"
	rowTmpl, err := Parse(rowSrc, "_row.hml", nil)
	if err != nil {
		b.Fatal(err)
	}

	src := "%ul\n" +
		"  - for row in rows\n" +
		"    = render \"row\", name: row.name, cls: row.cls, id: row.id\n"
	tmpl, err := Parse(src, "bench.hml", nil)
	if err != nil {
		b.Fatal(err)
	}

	rows := make([]any, 100)
	for i := range rows {
		rows[i] = map[string]any{
			"name": fmt.Sprintf("Row %d", i),
			"cls":  "row",
			"id":   int64(i),
		}
	}

	// Simulate a realistic layout locals map (~50 keys) that partials inherit.
	locals := map[string]any{"rows": rows}
	for i := range 50 {
		locals[fmt.Sprintf("layout_key_%d", i)] = i
	}

	// partialFn mirrors www: render the cached partial against the child
	// context the engine layered on the caller's, with no map copy.
	partialFn := func(name string, ctx *Context) (string, error) {
		return rowTmpl.RenderContext(ctx, nil)
	}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := tmpl.Render(locals, partialFn); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRenderPartialsLoopWriter is BenchmarkRenderPartialsLoop on
// the one-buffer path: the partial writes into the caller's builder
// rather than returning a string the caller copies.
func BenchmarkRenderPartialsLoopWriter(b *testing.B) {
	rowSrc := "%li{ class: cls, \"data-id\": id }\n" +
		"  = name\n"
	rowTmpl, err := Parse(rowSrc, "_row.hml", nil)
	if err != nil {
		b.Fatal(err)
	}

	src := "%ul\n" +
		"  - for row in rows\n" +
		"    = render \"row\", name: row.name, cls: row.cls, id: row.id\n"
	tmpl, err := Parse(src, "bench.hml", nil)
	if err != nil {
		b.Fatal(err)
	}

	rows := make([]any, 100)
	for i := range rows {
		rows[i] = map[string]any{
			"name": fmt.Sprintf("Row %d", i),
			"cls":  "row",
			"id":   int64(i),
		}
	}
	locals := map[string]any{"rows": rows}
	for i := range 50 {
		locals[fmt.Sprintf("layout_key_%d", i)] = i
	}

	partial := func(name string, ctx *Context, w *strings.Builder) error {
		return rowTmpl.RenderContextTo(w, ctx, nil)
	}

	b.ReportAllocs()
	for b.Loop() {
		var w strings.Builder
		if err := tmpl.RenderContextTo(&w, NewContext(locals), partial); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkToAnySlice measures the per-loop slice boxing for common concrete
// slice types that currently fall through to reflection.
func BenchmarkToAnySlice(b *testing.B) {
	maps := make([]map[string]any, 100)
	for i := range maps {
		maps[i] = map[string]any{"name": "x"}
	}
	strs := make([]string, 100)
	for i := range strs {
		strs[i] = "x"
	}
	b.Run("maps", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, ok := toAnySlice(maps); !ok {
				b.Fatal("not a slice")
			}
		}
	})
	b.Run("strings", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, ok := toAnySlice(strs); !ok {
				b.Fatal("not a slice")
			}
		}
	})
}

// BenchmarkRenderLoopConditional renders an if/else if/else chain per
// loop item, with a tag of each layout in the branches.
func BenchmarkRenderLoopConditional(b *testing.B) {
	src := "%ul\n" +
		"  - for row in rows\n" +
		"    - if row.id == 0\n" +
		"      %li.first\n" +
		"        = row.name\n" +
		"    - else if row.cls == \"x\"\n" +
		"      %li.x\n" +
		"    - else\n" +
		"      %li\n" +
		"        %span\n" +
		"          = row.name\n" +
		"        %br\n"
	tmpl, err := Parse(src, "bench.hml", nil)
	if err != nil {
		b.Fatal(err)
	}
	rows := make([]benchRow, 100)
	for i := range rows {
		rows[i] = benchRow{Name: fmt.Sprintf("Row %d", i), Cls: "row", ID: int64(i)}
	}
	locals := map[string]any{"rows": rows}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := tmpl.Render(locals, nil); err != nil {
			b.Fatal(err)
		}
	}
}

// benchPage is a whole page in the shape an app writes: a layout head,
// static navigation, a loop with dynamic attributes and a condition, a
// partial call, interpolated text, and a script filter.
const benchPage = `!!!
%html{ lang: "en" }
  %head
    %meta{ charset: "utf-8" }
    %title
      = title
    %link{ rel: "stylesheet", href: "/app.css" }
  %body.app
    %nav.top
      %a.brand{ href: "/" }
        Home
      %a{ href: "/companies" }
        Companies
      %a{ href: "/people" }
        People
    %main#content
      %h1
        Results for #{query}
      %table.rows
        %thead
          %tr
            %th
              Name
            %th
              Site
        %tbody
          - for i, row in rows
            %tr{ class: row.cls, "data-id": row.id }
              %td
                %a{ href: "/companies/show?id=#{row.id}" }
                  = row.name
              %td
                - if row.id == 0
                  %span.first
                    first
                - else
                  %a{ href: row.site, rel: "nofollow" }
                    = row.site
      = render "footer", year: year
    :javascript
      window.APP = { page: "#{title}" };
`

// benchSiteRow carries a URL from data, so each row exercises the URL
// scheme check in policyAttr.
type benchSiteRow struct {
	Name string `json:"name"`
	Cls  string `json:"cls"`
	Site string `json:"site"`
	ID   int64  `json:"id"`
}

func benchPageLocals() map[string]any {
	rows := make([]benchSiteRow, 100)
	for i := range rows {
		rows[i] = benchSiteRow{
			Name: fmt.Sprintf("Acme & Sons %d", i),
			Cls:  "row",
			Site: fmt.Sprintf("https://acme%d.example.com/about?ref=hml&x=%d", i, i),
			ID:   int64(i),
		}
	}
	return map[string]any{
		"title": "Companies",
		"query": "acme <inc>",
		"year":  int64(2026),
		"rows":  rows,
	}
}

// BenchmarkParsePage measures Parse on a whole page. An app parses each
// template once at startup, and a dev server parses on each change, so
// this is the cost of a cold start per template.
func BenchmarkParsePage(b *testing.B) {
	b.SetBytes(int64(len(benchPage)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Parse(benchPage, "page.hml", nil); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRenderPage renders benchPage with 100 rows through the
// one-buffer path, the way an HTTP handler does.
func BenchmarkRenderPage(b *testing.B) {
	tmpl, err := Parse(benchPage, "page.hml", nil)
	if err != nil {
		b.Fatal(err)
	}
	footer, err := Parse("%footer\n  %p\n    Copyright #{year}\n", "_footer.hml", nil)
	if err != nil {
		b.Fatal(err)
	}
	partial := func(name string, ctx *Context, w *strings.Builder) error {
		return footer.RenderContextTo(w, ctx, nil)
	}
	locals := benchPageLocals()
	var w strings.Builder
	if err := tmpl.RenderContextTo(&w, NewContext(locals), partial); err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(w.Len()))
	b.ReportAllocs()
	for b.Loop() {
		var w strings.Builder
		if err := tmpl.RenderContextTo(&w, NewContext(locals), partial); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRenderSplat splats a data map onto a tag per loop item. Each
// key goes through validAttrName, the sort, and the attribute policy at
// render, because a splat key is data.
func BenchmarkRenderSplat(b *testing.B) {
	src := "%ul\n" +
		"  - for row in rows\n" +
		"    %li{ **row }\n" +
		"      x\n"
	tmpl, err := Parse(src, "bench.hml", nil)
	if err != nil {
		b.Fatal(err)
	}
	rows := make([]any, 100)
	for i := range rows {
		rows[i] = map[string]any{
			"class":      "row",
			"data-id":    int64(i),
			"aria-label": fmt.Sprintf("Row %d", i),
			"title":      "A <b> title",
			"hidden":     false,
		}
	}
	locals := map[string]any{"rows": rows}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := tmpl.Render(locals, nil); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRenderURLAttrs renders URL attributes from data, half safe
// and half refused, so both branches of safeURL run per row.
func BenchmarkRenderURLAttrs(b *testing.B) {
	src := "%ul\n" +
		"  - for row in rows\n" +
		"    %li\n" +
		"      %a{ href: row.href }\n" +
		"        %img{ src: row.img, alt: row.name }\n"
	tmpl, err := Parse(src, "bench.hml", nil)
	if err != nil {
		b.Fatal(err)
	}
	rows := make([]any, 100)
	for i := range rows {
		href := fmt.Sprintf("https://example.com/companies/%d?tab=people&sort=name", i)
		if i%2 == 1 {
			href = "java\tscript:alert(1)"
		}
		rows[i] = map[string]any{
			"href": href,
			"img":  fmt.Sprintf("/img/%d.png", i),
			"name": fmt.Sprintf("Row %d", i),
		}
	}
	locals := map[string]any{"rows": rows}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := tmpl.Render(locals, nil); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRenderEscapedText renders long user text that holds markup
// characters, through output and through text interpolation, so
// escapeHTML does real work on each row.
func BenchmarkRenderEscapedText(b *testing.B) {
	src := "- for row in rows\n" +
		"  %p\n    = row.body\n" +
		"  %p\n" +
		"    By #{row.author} on #{row.date}\n"
	tmpl, err := Parse(src, "bench.hml", nil)
	if err != nil {
		b.Fatal(err)
	}
	body := strings.Repeat(`Tom & Jerry said "<hi>" and 'bye'. `, 20)
	rows := make([]any, 100)
	for i := range rows {
		rows[i] = map[string]any{
			"body":   body,
			"author": fmt.Sprintf("<user %d>", i),
			"date":   "2026-11-10",
		}
	}
	locals := map[string]any{"rows": rows}
	var w strings.Builder
	if err := tmpl.RenderContextTo(&w, NewContext(locals), nil); err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(w.Len()))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := tmpl.Render(locals, nil); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRenderNestedPartials renders a chain of 8 partials per loop
// item, each with an argument, so the depth check in renderPartialCall
// and the child Context run at each level. Real pages nest a few
// levels: a layout, a list, a row, an icon.
func BenchmarkRenderNestedPartials(b *testing.B) {
	const depth = 8
	templates := map[string]*Template{}
	for i := range depth {
		src := fmt.Sprintf("%%div{ class: \"level-%d\" }\n  = render \"p%d\", label: label\n", i, i+1)
		if i == depth-1 {
			src = "%span\n  = label\n"
		}
		tmpl, err := Parse(src, fmt.Sprintf("_p%d.hml", i), nil)
		if err != nil {
			b.Fatal(err)
		}
		templates[fmt.Sprintf("p%d", i)] = tmpl
	}
	page, err := Parse("- for row in rows\n  = render \"p0\", label: row\n", "page.hml", nil)
	if err != nil {
		b.Fatal(err)
	}
	var partial PartialWriter
	partial = func(name string, ctx *Context, w *strings.Builder) error {
		tmpl, ok := templates[name]
		if !ok {
			return fmt.Errorf("unknown partial %q", name)
		}
		return tmpl.RenderContextTo(w, ctx, partial)
	}
	rows := make([]string, 20)
	for i := range rows {
		rows[i] = fmt.Sprintf("Row %d", i)
	}
	locals := map[string]any{"rows": rows}
	b.ReportAllocs()
	for b.Loop() {
		var w strings.Builder
		if err := page.RenderContextTo(&w, NewContext(locals), partial); err != nil {
			b.Fatal(err)
		}
	}
}
