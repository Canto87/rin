package main

import "testing"

func TestClassifyLevel(t *testing.T) {
	tests := []struct {
		name            string
		fileCount       int
		hasDependencies bool
		needsDesign     bool
		want            string
	}{
		{
			name:            "zero files no deps no design",
			fileCount:       0,
			hasDependencies: false,
			needsDesign:     false,
			want:            "L1",
		},
		{
			name:            "one file",
			fileCount:       1,
			hasDependencies: false,
			needsDesign:     false,
			want:            "L1",
		},
		{
			name:            "two files",
			fileCount:       2,
			hasDependencies: false,
			needsDesign:     false,
			want:            "L2",
		},
		{
			name:            "three files",
			fileCount:       3,
			hasDependencies: false,
			needsDesign:     false,
			want:            "L2",
		},
		{
			name:            "four files promotes to L3",
			fileCount:       4,
			hasDependencies: false,
			needsDesign:     false,
			want:            "L3",
		},
		{
			name:            "has dependencies forces L3",
			fileCount:       0,
			hasDependencies: true,
			needsDesign:     false,
			want:            "L3",
		},
		{
			name:            "needs design forces L3",
			fileCount:       0,
			hasDependencies: false,
			needsDesign:     true,
			want:            "L3",
		},
		{
			name:            "both deps and design forces L3",
			fileCount:       1,
			hasDependencies: true,
			needsDesign:     true,
			want:            "L3",
		},
		{
			name:            "high file count with deps",
			fileCount:       10,
			hasDependencies: true,
			needsDesign:     false,
			want:            "L3",
		},
		{
			name:            "boundary: exactly three files no flags",
			fileCount:       3,
			hasDependencies: false,
			needsDesign:     false,
			want:            "L2",
		},
		{
			name:            "one file with design",
			fileCount:       1,
			hasDependencies: false,
			needsDesign:     true,
			want:            "L3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyLevel(tt.fileCount, tt.hasDependencies, tt.needsDesign)
			if got != tt.want {
				t.Errorf("classifyLevel(%d, %v, %v) = %q, want %q",
					tt.fileCount, tt.hasDependencies, tt.needsDesign, got, tt.want)
			}
		})
	}
}
