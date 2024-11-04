package flags

import (
	"fmt"
	"github.com/Project-Faster/monkey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"os"
	"strings"
	"testing"
)

func TestFlagsSuite(t *testing.T) {
	var q FlagsSuite
	suite.Run(t, &q)
}

type FlagsSuite struct{ suite.Suite }

func (s *FlagsSuite) AfterTest(_, _ string) {
	monkey.UnpatchAll()
}

func (s *FlagsSuite) BeforeTest(_, _ string) {
	Globals = GlobalFlags{}
}

func (s *FlagsSuite) TestParseFlags_Empty() {
	t := s.T()

	ParseFlags([]string{})

	assert.Equal(t, "", Globals.Service)
	assert.False(t, Globals.Client)
	assert.False(t, Globals.Verbose)

	assert.NotNil(t, Globals.ConfigOverrideCallback)
	assert.Len(t, Globals.ConfigOverrides, 0)
}

func (s *FlagsSuite) TestParseFlags_Combinations() {
	t := s.T()

	var flaglist = [][]string{
		{"--service", "start"},
		{"--client"},
		{"--verbose"},
		{""},
		{""},
		{""},
	}

	for i := 0; i < 6; i++ {
		for j := 0; j < 6; j++ {
			if i == j {
				continue
			}
			for k := 0; k < 6; k++ {
				if i == k || j == k {
					continue
				}

				var list = []string{}
				list = append(list, flaglist[i]...)
				list = append(list, flaglist[j]...)
				list = append(list, flaglist[k]...)
				t.Logf("check: %v\n", list)

				ParseFlags(list)

				cmd := fmt.Sprintf("%v", list)

				if strings.Index(cmd, "--service") != -1 {
					assert.Equal(t, "start", Globals.Service)
				} else {
					assert.Equal(t, "", Globals.Service)
				}
				if strings.Index(cmd, "--client") != -1 {
					assert.True(t, Globals.Client)
				} else {
					assert.False(t, Globals.Client)
				}
				if strings.Index(cmd, "--verbose") != -1 {
					assert.True(t, Globals.Verbose)
				} else {
					assert.False(t, Globals.Verbose)
				}

				assert.NotNil(t, Globals.ConfigOverrideCallback)
				assert.Len(t, Globals.ConfigOverrides, 0)
			}
		}
	}
}

func (s *FlagsSuite) TestParseFlags_SyntaxError() {
	t := s.T()

	exitCode := 0
	monkey.Patch(os.Exit, func(code int) {
		exitCode = code
	})

	for _, err := range []string{"--serv", "--cl", "--verb"} {
		exitCode = 0
		ParseFlags([]string{err})

		assert.Equal(t, 1, exitCode)
	}
}

func (s *FlagsSuite) TestParseFlags_Overrides() {
	t := s.T()

	ParseFlags([]string{"-Dcheck=value", "-Dtest=val2", "-Dtest2=1"})

	assert.NotNil(t, Globals.ConfigOverrides)

	value, ok := Globals.ConfigOverrides["check"]
	assert.True(t, ok)
	assert.Equal(t, "value", value)

	value, ok = Globals.ConfigOverrides["test"]
	assert.True(t, ok)
	assert.Equal(t, "val2", value)

	value, ok = Globals.ConfigOverrides["test2"]
	assert.True(t, ok)
	assert.Equal(t, "1", value)
}

func (s *FlagsSuite) TestParseFlags_OverridesFail() {
	t := s.T()

	ParseFlags([]string{"-Dcheck=value", "-Dtestval2", "-Dtest2=1"})

	assert.NotNil(t, Globals.ConfigOverrides)

	value, ok := Globals.ConfigOverrides["check"]
	assert.True(t, ok)
	assert.Equal(t, "value", value)

	_, ok = Globals.ConfigOverrides["test"]
	assert.False(t, ok)

	value, ok = Globals.ConfigOverrides["test2"]
	assert.True(t, ok)
	assert.Equal(t, "1", value)
}
