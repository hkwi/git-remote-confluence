package confluence

import "testing"

func TestSafeAttachmentName(t *testing.T) {
	tests := []struct {
		title string
		id    string
		want  string
	}{
		{title: "diagram.png", id: "10", want: "diagram.png"},
		{title: "../../secret", id: "11", want: ".._.._secret"},
		{title: "a\\b\n.txt", id: "12", want: "a_b_.txt"},
		{title: "..", id: "13", want: "attachment-13"},
		{title: "", id: "14", want: "attachment-14"},
	}

	for _, test := range tests {
		if got := safeAttachmentName(test.title, test.id); got != test.want {
			t.Errorf("safeAttachmentName(%q, %q) = %q, want %q", test.title, test.id, got, test.want)
		}
	}
}
