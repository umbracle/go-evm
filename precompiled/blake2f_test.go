package precompiled

import (
	"bytes"
	"testing"
)

func TestBlake2f(t *testing.T) {
	b := &Blake2f{}

	// TODO: Use this for all the precompiled test cases
	ReadTestCase(t, "blake2f.json", func(t *testing.T, c *TestCase) {
		out, err := b.Run(c.Input)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(c.Expected, out) {
			t.Fatal("bad")
		}
	})
}
