package ui

import (
	"reflect"
	"testing"
)

func completionLabels(items []CompletionItem) []string {
	labels := make([]string, len(items))
	for i, it := range items {
		labels[i] = it.Label
	}
	return labels
}

func itemsWithSortText(sortText string, labels ...string) []CompletionItem {
	items := make([]CompletionItem, len(labels))
	for i, l := range labels {
		items[i] = CompletionItem{Label: l, SortText: sortText}
	}
	return items
}

func TestFilterCompletionsOrder(t *testing.T) {
	tests := []struct {
		name   string
		items  []CompletionItem
		prefix string
		want   []string
	}{
		{
			name:   "same case prefix wins a sortText tie",
			items:  itemsWithSortText("15", "StringIterator", "String", "StructuredSerializeOptions", "StreamPipeOptions", "StringConstructor", "string"),
			prefix: "str",
			want:   []string{"string", "String", "StringIterator", "StreamPipeOptions", "StringConstructor", "StructuredSerializeOptions"},
		},
		{
			name:   "lowercase keywords before types",
			items:  itemsWithSortText("15", "NodeIterator", "number", "never", "null", "NonNullable"),
			prefix: "n",
			want:   []string{"null", "never", "number", "NonNullable", "NodeIterator"},
		},
		{
			name: "lower sortText beats a case match",
			items: []CompletionItem{
				{Label: "string", SortText: "15"},
				{Label: "Str", SortText: "10"},
			},
			prefix: "str",
			want:   []string{"Str", "string"},
		},
		{
			name:   "empty sortText uses the same tie-breaks",
			items:  itemsWithSortText("", "StringIterator", "String", "const", "string", "console"),
			prefix: "",
			want:   []string{"console", "const", "String", "string", "StringIterator"},
		},
		{
			name:   "empty sortText with a typed prefix",
			items:  itemsWithSortText("", "StringIterator", "String", "string"),
			prefix: "str",
			want:   []string{"string", "String", "StringIterator"},
		},
		{
			name: "empty prefix sorts everything by sortText then label",
			items: []CompletionItem{
				{Label: "toString", SortText: "11"},
				{Label: "length", SortText: "11"},
				{Label: "at", SortText: "11"},
				{Label: "local", SortText: "10"},
			},
			prefix: "",
			want:   []string{"local", "at", "length", "toString"},
		},
		{
			name: "filterText drives the case check",
			items: []CompletionItem{
				{Label: "aaa", FilterText: "String", SortText: "15"},
				{Label: "bbb", FilterText: "string", SortText: "15"},
			},
			prefix: "str",
			want:   []string{"bbb", "aaa"},
		},
		{
			name:   "non matching items are still dropped",
			items:  itemsWithSortText("15", "string", "number"),
			prefix: "str",
			want:   []string{"string"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := completionLabels(FilterCompletions(tt.items, tt.prefix))
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFilterCompletionsDoesNotReorderInput(t *testing.T) {
	items := itemsWithSortText("15", "b", "a")
	FilterCompletions(items, "")
	if got := completionLabels(items); !reflect.DeepEqual(got, []string{"b", "a"}) {
		t.Fatalf("input reordered: %v", got)
	}
}
