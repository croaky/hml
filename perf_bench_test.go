package hml

import (
	"fmt"
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
