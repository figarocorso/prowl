package data

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSummarizeDiff(t *testing.T) {
	diff := `diff --git a/foo.go b/foo.go
index 1111..2222 100644
--- a/foo.go
+++ b/foo.go
@@ -1,3 +1,4 @@
 unchanged
-removed
+added one
+added two
diff --git a/bar.go b/bar.go
--- a/bar.go
+++ b/bar.go
@@ -10,1 +10,1 @@
-old line
+new line
`
	s := SummarizeDiff(diff)
	assert.Equal(t, 2, s.Files)
	assert.Equal(t, 3, s.Additions)
	assert.Equal(t, 2, s.Deletions)
}

func TestSummarizeDiffEmpty(t *testing.T) {
	s := SummarizeDiff("")
	assert.Equal(t, 0, s.Files)
	assert.Equal(t, 0, s.Additions)
	assert.Equal(t, 0, s.Deletions)
}

func TestSummarizeDiffHeaderLinesIgnored(t *testing.T) {
	diff := "diff --git a/x b/x\n--- a/x\n+++ b/x\n+real add\n"
	s := SummarizeDiff(diff)
	assert.Equal(t, 1, s.Files)
	assert.Equal(t, 1, s.Additions)
	assert.Equal(t, 0, s.Deletions)
}
