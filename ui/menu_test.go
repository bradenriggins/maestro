package ui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"maestro/session"
)

func TestMenuOmitsEnterForLoadingInstance(t *testing.T) {
	menu := NewMenu()
	menu.SetSize(100, 1)

	instance, err := session.NewInstance(session.InstanceOptions{
		Title:   "loading-session",
		Path:    t.TempDir(),
		Program: "claude",
	})
	require.NoError(t, err)
	instance.SetStatus(session.Loading)

	menu.SetInstance(instance)

	rendered := menu.String()

	require.False(t, strings.Contains(rendered, "Enter open"), "loading sessions should not advertise the dead Enter action")
}
