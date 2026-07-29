// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// positiveDurationValidator validates that a string attribute holds a Go
// duration string (as understood by time.ParseDuration) with a strictly
// positive value. Several attributes across the provider are documented as "Go
// duration string" but are only parsed much later — on a helper daemon, or when
// the wait loop starts — so catching a typo at plan time is much friendlier than
// failing mid-apply.
type positiveDurationValidator struct{}

var _ validator.String = positiveDurationValidator{}

func (v positiveDurationValidator) Description(_ context.Context) string {
	return "must be a positive Go duration string, e.g. \"30s\", \"5m\", \"1h30m\""
}

func (v positiveDurationValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v positiveDurationValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	// A null value means the attribute is unset (a Default, if any, is applied
	// after validation) and an unknown one is only resolved during apply;
	// neither is something this validator can or should judge.
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	s := req.ConfigValue.ValueString()

	d, err := time.ParseDuration(s)
	if err != nil {
		resp.Diagnostics.AddAttributeError(req.Path,
			"Invalid Duration",
			fmt.Sprintf("%q is not a valid Go duration string: %s\n\n"+
				"A duration is a number followed by a unit, optionally repeated, e.g. \"30s\", \"5m\", \"1h30m\". "+
				"Valid units are \"ns\", \"us\", \"ms\", \"s\", \"m\" and \"h\"; a bare number such as \"30\" is not valid.",
				s, err))
		return
	}

	if d <= 0 {
		resp.Diagnostics.AddAttributeError(req.Path,
			"Invalid Duration",
			fmt.Sprintf("%q must be a positive duration, got %s.", s, d))
	}
}
