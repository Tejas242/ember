package island

import "testing"

func TestSwizzle(t *testing.T) {
	src := []byte{1, 2, 3, 4, 10, 20, 30, 40}
	dst := make([]byte, 8)
	swizzle(dst, src)
	if string(dst) != string([]byte{3, 2, 1, 4, 30, 20, 10, 40}) {
		t.Fatal(dst)
	}
}
