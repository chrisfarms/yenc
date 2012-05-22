package yenc

import (
	"os"
	"testing"
)

func TestSinglepartDecode(t *testing.T) {
	f, err := os.Open("singlepart_test.yenc")
	if err != nil {
		t.Fatal("could not open singlepart_test.yenc for testing")
	}
	_, err = Decode(f)
	if err != nil {
		t.Fatal("expected to decode: " + err.Error())
	}
}

func TestMultipartDecode(t *testing.T) {
	f, err := os.Open("multipart_test.yenc")
	if err != nil {
		t.Fatal("could not open multipart_test.yenc for testing")
	}
	part, err := Decode(f)
	if err != nil {
		t.Errorf("expected to decode: " + err.Error())
	}
	if part.Name != "joystick.jpg" {
		t.Errorf("expected part name %s got %s", "joystick.jpg", part.Name)
	}
	// out,_ := os.Create("joystick.jpg")
	// out.Write(part.Body)
}
