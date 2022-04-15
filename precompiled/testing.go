package precompiled

import (
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"testing"
)

type TestCase struct {
	Name     string
	Input    []byte
	Expected []byte
	Gas      uint64
}

func ReadTestCase(t *testing.T, path string, f func(t *testing.T, c *TestCase)) {
	data, err := ioutil.ReadFile(filepath.Join("./fixtures", path))
	if err != nil {
		t.Fatal(err)
	}

	type testCase struct {
		Name     string
		Input    string
		Expected string
		Gas      uint64
	}
	var cases []*testCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}

	for _, i := range cases {
		input, _ := hex.DecodeString(i.Input)
		expected, _ := hex.DecodeString(i.Expected)

		c := &TestCase{
			Name:     i.Name,
			Gas:      i.Gas,
			Input:    input,
			Expected: expected,
		}
		t.Run(i.Name, func(t *testing.T) {
			f(t, c)
		})
	}
}
