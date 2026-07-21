// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"runtime"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/matryer/is"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/exec"
)

// stubExec embeds exec.Executor (left nil) and only overrides IsLocal, so it can
// exercise the hypervisor-selection routing without a real target. Any other
// method call would panic, which is fine: the tested branches only call
// IsLocal (or return before touching the executor).
type stubExec struct {
	exec.Executor
	local bool
}

func (s stubExec) IsLocal() bool { return s.local }

func TestResolveTargetPlatform_Local(t *testing.T) {
	is := is.New(t)

	// A LocalExecutor runs uname on the test host, so the detected platform
	// must match this process's GOOS/GOARCH.
	goos, goarch, err := resolveTargetPlatform(context.Background(), exec.NewLocal(false))
	is.NoErr(err)
	is.Equal(goos, runtime.GOOS)
	is.Equal(goarch, runtime.GOARCH)
}

func TestIsDevVersion(t *testing.T) {
	is := is.New(t)

	is.True(isDevVersion(""))
	is.True(isDevVersion("dev"))
	is.True(isDevVersion("test"))
	is.True(!isDevVersion("0.9.0"))
	is.True(!isDevVersion("v1.2.3"))
}

func TestConfigurePlatformTools_Routing(t *testing.T) {
	t.Run("remote darwin target is rejected", func(t *testing.T) {
		is := is.New(t)
		zaConf := &ZedAmigoProviderConfig{TargetOS: "darwin", Exec: stubExec{local: false}}
		resp := &provider.ConfigureResponse{}
		configurePlatformTools(context.Background(), zaConf, resp)
		is.True(resp.Diagnostics.HasError()) // remote macOS is not supported
	})

	t.Run("unsupported target OS is rejected", func(t *testing.T) {
		is := is.New(t)
		zaConf := &ZedAmigoProviderConfig{TargetOS: "plan9", Exec: stubExec{local: false}}
		resp := &provider.ConfigureResponse{}
		configurePlatformTools(context.Background(), zaConf, resp)
		is.True(resp.Diagnostics.HasError())
	})
}
