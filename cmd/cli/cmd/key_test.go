package cmd

import (
	"strings"
	"testing"
)

func TestImportMneumonic(t *testing.T) {
	mneumonic := "yellow yellow yellow yellow yellow yellow yellow yellow yellow yellow yellow yellow"

	ImportMneumonic("seed", strings.Fields(mneumonic))

}
