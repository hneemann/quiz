package server

import "testing"

func Test_getStrFromPath(t *testing.T) {
	tests := []struct {
		path string
		v    string
		n    string
	}{
		{"/task/2/3", "3", "/task/2"},
		{"/task/2/3/", "3", "/task/2"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			v, n := getStrFromPath(tt.path)
			if v != tt.v {
				t.Errorf("getStrFromPath() got = %v, want %v", v, tt.v)
			}
			if n != tt.n {
				t.Errorf("getStrFromPath() got1 = %v, want %v", n, tt.n)
			}
		})
	}
}
