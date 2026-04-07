package terminalui

import "testing"

func TestResizeInputTracksLineCountWithCap(t *testing.T) {
	model := NewModel(nil)

	model.input.SetValue("one")
	model.resizeInput()
	if model.input.Height() != 1 {
		t.Fatalf("height = %d", model.input.Height())
	}

	model.input.SetValue("one\ntwo\nthree")
	model.resizeInput()
	if model.input.Height() != 3 {
		t.Fatalf("height = %d", model.input.Height())
	}

	model.input.SetValue("1\n2\n3\n4\n5\n6\n7\n8")
	model.resizeInput()
	if model.input.Height() != 6 {
		t.Fatalf("height = %d", model.input.Height())
	}
}
