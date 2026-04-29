package object

import (
	"os"
	"testing"

	"github.com/lczyk/assert"
)

func TestFileExpandPath(t *testing.T) {
	t.Run("one arg flavour", func(t *testing.T) {
		env := NewEnvironment()
		context := &callContext{
			receiver: fileClass,
			env:      env,
		}
		filename := NewString("./fixtures/testfile.rb")

		result, err := fileExpandPath(context, nil, filename)

		assert.NoError(t, err)

		cwd, err := os.Getwd()
		if err != nil {
			t.Skip("Cannot determine working directory")
		}
		expected := NewString(cwd + "/fixtures/testfile.rb")

		assert.EqualCmpAny(t, result, expected, CompareRubyObjectsForTests)
	})
	t.Run("two arg flavour", func(t *testing.T) {
		env := NewEnvironment()
		context := &callContext{
			receiver: fileClass,
			env:      env,
		}
		filename := NewString("../../main.go")
		dirname := NewString("object/fixtures/")

		result, err := fileExpandPath(context, nil, filename, dirname)

		assert.NoError(t, err)

		cwd, err := os.Getwd()
		if err != nil {
			t.Skip("Cannot determine working directory")
		}
		expected := NewString(cwd + "/main.go")

		assert.EqualCmpAny(t, result, expected, CompareRubyObjectsForTests)
	})
}

func TestFileDirname(t *testing.T) {
	context := &callContext{
		receiver: fileClass,
		env:      NewEnvironment(),
	}
	filename := NewString("/var/log/foo.log")

	result, err := fileDirname(context, nil, filename)

	assert.NoError(t, err)

	expected := NewString("/var/log")

	assert.EqualCmpAny(t, result, expected, CompareRubyObjectsForTests)
}
