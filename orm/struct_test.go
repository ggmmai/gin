package orm

import (
	"database/sql"
	"reflect"
	"testing"
	"unsafe"
)

func TestStructMeta(t *testing.T) {
	type inner struct {
		ID int32 `orm:"id"`
	}
	type row struct {
		inner
		InstID string `orm:"instId"`
		Name   string
		Skip   int `orm:"-"`
		secret string
		Dup    int `orm:"id"`
	}
	meta, err := cachedStructMeta(reflect.TypeFor[row]())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := meta.fields["id"]; !ok {
		t.Fatal("missing id")
	}
	if meta.fields["id"].index[0] != 0 {
		t.Fatalf("first id field should win, index %v", meta.fields["id"].index)
	}
	if _, ok := meta.fields["instId"]; !ok {
		t.Fatal("missing instId")
	}
	if _, ok := meta.fields["Name"]; !ok {
		t.Fatal("missing Name")
	}
	if _, ok := meta.fields["Skip"]; ok {
		t.Fatal("skipped field was included")
	}
	if _, ok := meta.fields["secret"]; ok {
		t.Fatal("unexported field was included")
	}

	if _, err := cachedStructMeta(reflect.TypeFor[int]()); err == nil {
		t.Fatal("expected non-struct error")
	}
	if _, err := cachedStructMeta(reflect.TypeFor[*row]()); err == nil {
		t.Fatal("expected pointer error")
	}
}

func TestGetIntoRejectsBadQuery(t *testing.T) {
	type row struct {
		ID int32 `orm:"id"`
	}
	var b *Builder
	if _, err := b.GetInto[row](); err == nil {
		t.Fatal("expected nil builder error")
	}
	if _, err := (&Builder{}).GetInto[int](); err == nil {
		t.Fatal("expected non-struct error")
	}
	if _, err := (&Builder{}).GetInto[row](); err == nil {
		t.Fatal("expected nil database error")
	}
	if _, err := (&Builder{}).FindInto[row](); err == nil {
		t.Fatal("expected nil database error")
	}
}

func TestBindStructFields(t *testing.T) {
	type inner struct {
		ID int32 `orm:"id"`
	}
	type row struct {
		inner
		Name  string         `orm:"name"`
		Title sql.NullString `orm:"title"`
	}
	meta, err := cachedStructMeta(reflect.TypeFor[row]())
	if err != nil {
		t.Fatal(err)
	}
	if meta.fields["id"].scanner || meta.fields["name"].scanner {
		t.Fatal("plain fields should scan directly")
	}
	if !meta.fields["title"].scanner {
		t.Fatal("NullString should be marked as Scanner")
	}

	var got row
	layout := columnLayout{
		keys:  []string{"id", "name", "extra"},
		kinds: []valueKind{kindInt32, kindString, kindString},
	}
	binds, ptrs, holes := newColBinds(layout, meta)
	bindStructFields(unsafe.Pointer(&got), binds, ptrs, holes)
	*ptrs[0].(*int32) = 7
	*ptrs[1].(*string) = "BTC"
	if got.ID != 7 || got.Name != "BTC" {
		t.Fatalf("%+v", got)
	}
	p, ok := ptrs[2].(*any)
	if !ok || p != &holes[2] {
		t.Fatalf("unmatched column destination %T", ptrs[2])
	}
}
