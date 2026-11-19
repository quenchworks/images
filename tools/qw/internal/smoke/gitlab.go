package smoke

import "regexp"

func init() {
	// gitlab-rails: first-stage check without a database. The image must run as git (1001), load Rails 8.0 through
	// bundler, and load every native gem the recipe compiles from source (a missing Wolfi library fails here, not at
	// the first request). A full boot needs PostgreSQL, Redis and Gitaly: the chart gate covers that.
	register("gitlab-rails", func(t *T) {
		t.User("1001")
		v := t.Match(t.Run("rails", "--version"), `Rails (8\.0\.[0-9.]+)`, "rails --version")
		if t.Want != "" {
			t.Match(t.RunEntry("cat", "/srv/gitlab/VERSION"), `^`+regexp.QuoteMeta(t.Want)+`\s*$`, "VERSION file")
		}
		gems := `%w[nokogiri grpc re2 pg google/protobuf ffi prometheus/client/formats/text gitlab_glfm_markdown rugged gpgme charlock_holmes oj unf_ext].each { |g| require g }; puts "NATIVE_OK"`
		t.Match(t.Run("ruby", "-e", `require "bundler/setup"; `+gems), `NATIVE_OK`, "native gems")
		t.Logf("smoke test passed (uid 1001, Rails %s, native gems load)", v)
	})
}
