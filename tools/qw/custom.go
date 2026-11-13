package main

import (
	"strings"
)

const calicoNode = `^calico-node-\d+\.\d+$`

var endoflifeLatest = func(d any) []string {
	return each(d, func(c any) string { return str(path(c, "latest")) })
}

func init() {
	for app, c := range map[string]check{
		"actual":                          {src: npmPkg("@actual-app/sync-server"), n: 1, f: filter{keep: noDash}},
		"airflow":                         {src: pypiPkg("apache-airflow"), lines: 2, f: filter{keep: numDots}},
		"ansible":                         {src: pypiPkg("ansible"), n: 2, f: filter{keep: numDots}},
		"apache-pinot":                    {src: ghRel("apache/pinot"), n: 1, f: group(`(?:release-)?(\d+\.\d+\.\d+)`)},
		"apache-polaris":                  {src: ghRel("apache/polaris"), n: 1, f: group(`apache-polaris-(\d+\.\d+\.\d+)`)},
		"aspnet":                          {run: reportWolfiSuffix("aspnet", `^aspnet-\d+-runtime$`, "aspnet-{maj}-runtime")},
		"dotnet-runtime":                  {run: reportWolfiSuffix("dotnet-runtime", `^dotnet-\d+-runtime$`, "dotnet-{maj}-runtime")},
		"dotnet":                          {run: reportWolfiSuffix("dotnet", `^dotnet-\d+-sdk$`, "dotnet-{maj}-sdk")},
		"backdrop":                        {src: ghRel("backdrop/backdrop"), lines: 2},
		"buildah":                         {src: ghRel("containers/buildah"), lines: 2},
		"fulcio":                          {src: ghRel("sigstore/fulcio"), lines: 2},
		"joomla":                          {src: ghRel("joomla/joomla-cms"), lines: 2},
		"matomo":                          {src: ghRel("matomo-org/matomo"), lines: 2},
		"percona-xtradb-cluster-operator": {src: ghRel("percona/percona-xtradb-cluster-operator"), lines: 2},
		"podman":                          {src: ghRel("containers/podman"), lines: 2},
		"rekor-tiles":                     {src: ghRel("sigstore/rekor-tiles"), lines: 2},
		"timestamp-authority":             {src: ghRel("sigstore/timestamp-authority"), lines: 2},
		"calico-cni":                      {src: wolfiVals(calicoNode), n: 1},
		"calico-csi":                      {src: wolfiVals(calicoNode), n: 1},
		"calico-key-cert-provisioner":     {src: wolfiVals(calicoNode), n: 1},
		"calico-kube-controllers":         {src: wolfiVals(calicoNode), n: 1},
		"calico-node-driver-registrar":    {src: wolfiVals(calicoNode), n: 1},
		"calico-node":                     {src: wolfiVals(calicoNode), n: 1},
		"calico-pod2daemon-flexvol":       {src: wolfiVals(calicoNode), n: 1},
		"calico-typha":                    {src: wolfiVals(calicoNode), n: 1},
		"clamav":                          {src: ghRel("Cisco-Talos/clamav"), lines: 2, f: filter{keep: prefix("clamav-"), clean: trim("clamav-")}},
		"clickhouse": {src: ghRel("ClickHouse/ClickHouse"), lines: 2, f: filter{
			keep:  func(t string) bool { return strings.Contains(t, "-stable") },
			clean: func(t string) string { return strings.SplitN(t, "-", 2)[0] }}},
		"coolify-app": {src: ghRel("coollabsio/coolify"), n: 1, f: filter{keep: full(`4\.\d+\.\d+`)}},
		"coolify-helper": {src: jsonList("https://raw.githubusercontent.com/coollabsio/coolify/main/upstream.json",
			func(d any) []string { return []string{str(path(d, "helper", "version"))} })},
		"couchdb":     {src: ghTags("apache/couchdb"), f: filter{keep: full(`\d+\.\d+\.\d+`)}},
		"haproxy":     {src: ghTags("haproxy/haproxy"), f: filter{keep: full(`\d+\.\d+\.\d+`)}},
		"memcached":   {src: ghTags("memcached/memcached"), f: filter{keep: full(`\d+\.\d+\.\d+`)}},
		"mosquitto":   {src: ghTags("eclipse/mosquitto"), f: filter{keep: full(`\d+\.\d+\.\d+`)}},
		"descheduler": {src: ghRel("kubernetes-sigs/descheduler"), n: 1, f: filter{keep: notPrefix("descheduler-helm-chart")}},
		"documentdb": {src: ghTags("microsoft/documentdb"), f: filter{keep: full(`\d+\.\d+-\d+`),
			clean: func(t string) string { return strings.ReplaceAll(t, "-", ".") }}},
		"filebrowser-quantum": {src: ghRel("gtsteffaniak/filebrowser"), n: 1, f: filter{
			keep:  func(t string) bool { return strings.HasSuffix(t, "-stable") },
			clean: func(t string) string { return strings.TrimSuffix(t, "-stable") }}},
		"flyway":    {src: ghRel("flyway/flyway"), n: 1, f: mapThen(trim("flyway-"), full(`\d+\.\d+\.\d+`))},
		"unbound":   {src: ghRel("NLnetLabs/unbound"), n: 1, f: mapThen(trim("release-"), full(`\d+\.\d+\.\d+`))},
		"friendica": {src: scrapeSrc("https://files.friendi.ca/", `friendica-full-(\d{4}\.\d{2}(?:-\d+)?)\.tar\.gz"`), n: 1},
		"garage": {src: jsonList("https://git.deuxfleurs.fr/api/v1/repos/Deuxfleurs/garage/tags?limit=50",
			func(d any) []string {
				return each(d, func(t any) string { return strings.TrimLeft(str(path(t, "name")), "v") })
			}), lines: 2},
		"ghost":    {src: npmPkg("ghost"), f: filter{keep: noDash}},
		"renovate": {src: npmPkg("renovate"), f: filter{keep: noDash}},
		"pnpm":     {src: npmPkg("pnpm"), lines: 1, f: filter{keep: noDash}},
		"gitlab-runner": {src: jsonList("https://gitlab.com/api/v4/projects/gitlab-org%2Fgitlab-runner/releases?per_page=40",
			func(d any) []string {
				return each(d, func(r any) string {
					if b, _ := path(r, "upcoming_release").(bool); b {
						return ""
					}
					return strings.TrimLeft(str(path(r, "tag_name")), "v")
				})
			})},
		"gravitee-console":          graviteeCheck,
		"gravitee-gateway":          graviteeCheck,
		"gravitee-management-api":   graviteeCheck,
		"guacamole":                 {src: ghTags("apache/guacamole-client"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
		"guacd":                     {src: ghTags("apache/guacamole-server"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
		"hasura":                    {src: ghRel("hasura/graphql-engine"), lines: 2, f: filter{keep: prefix("2.")}},
		"influxdb":                  {src: ghRel("influxdata/influxdb"), f: filter{keep: prefix("2.")}},
		"jaeger":                    {src: ghRel("jaegertracing/jaeger"), n: 1, f: filter{keep: prefix("2.")}},
		"metabase":                  {src: ghRel("metabase/metabase"), n: 1, f: filter{keep: prefix("0.")}},
		"jmeter":                    {src: ghRel("apache/jmeter"), f: filter{clean: func(t string) string { return strings.ReplaceAll(t, "rel/v", "") }}},
		"linkerd-cni":               {src: ghRel("linkerd/linkerd2-proxy-init"), n: 1, f: filter{keep: prefix("cni-plugin/v"), clean: trim("cni-plugin/v")}},
		"linkerd-extension-init":    {src: ghRel("linkerd/linkerd-extension-init"), n: 1, f: filter{keep: prefix("release/v"), clean: trim("release/v")}},
		"linkerd-control-plane":     linkerdEdge,
		"linkerd-policy-controller": linkerdEdge,
		"linkerd-viz-metrics-api":   linkerdEdge,
		"linkerd-viz-tap":           linkerdEdge,
		"mariadb-operator":          {src: ghRel("mariadb-operator/mariadb-operator"), lines: 2, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
		"metallb-controller":        {src: ghRel("metallb/metallb"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
		"metallb-speaker":           {src: ghRel("metallb/metallb"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
		"metrics-server":            {src: ghRel("kubernetes-sigs/metrics-server"), n: 1, f: filter{keep: notPrefix("metrics-server-helm-chart")}},
		"mimir":                     {src: ghRel("grafana/mimir"), f: filter{clean: trim("mimir-")}},
		"mlflow":                    {src: pypiPkg("mlflow"), f: filter{keep: numDots}},
		"vllm":                      {src: pypiPkg("vllm"), f: filter{keep: numDots}},
		"mongo-express": {src: jsonList("https://registry.npmjs.org/mongo-express",
			func(d any) []string { return []string{str(path(d, "dist-tags", "latest"))} }), n: 1},
		"mysql":  {src: jsonList("https://endoflife.date/api/mysql.json", endoflifeLatest), lines: 2},
		"neo4j":  {src: ghTags("neo4j/neo4j"), lines: 2, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
		"nessie": {src: ghRel("projectnessie/nessie"), n: 1, f: group(`nessie-(\d+\.\d+\.\d+)`)},
		"nifi":   {src: ghTags("apache/nifi"), n: 1, f: group(`rel/nifi-(\d+\.\d+\.\d+)`)},
		"odoo": {src: func() ([]string, error) {
			ds, err := scrape("https://nightly.odoo.com/19.0/nightly/src/", `odoo_19\.0\.([0-9]{8})\.tar\.gz`)
			for i := range ds {
				ds[i] = "19.0." + ds[i]
			}
			return ds, err
		}, n: 1},
		"openresty": {src: ghTags("openresty/openresty"), lines: 2, f: filter{keep: full(`\d+\.\d+\.\d+\.\d+`)}},
		"pgbouncer": {src: ghRel("pgbouncer/pgbouncer"), f: filter{clean: func(t string) string {
			return strings.ReplaceAll(strings.ReplaceAll(t, "pgbouncer_", ""), "_", ".")
		}}},
		"phpmyadmin": {src: jsonList("https://www.phpmyadmin.net/home_page/version.json",
			func(d any) []string { return []string{str(path(d, "version"))} }), n: 1},
		"postgres-documentdb": {src: ghRel("FerretDB/documentdb"), f: filter{clean: func(t string) string { return strings.SplitN(t, "-ferretdb", 2)[0] }}},
		"reloader":            {src: ghRel("stakater/Reloader"), n: 1, f: filter{keep: notPrefix("chart")}},
		"rustfs": {src: ghAssets("rustfs/rustfs"), f: filter{keep: func(v string) bool {
			for _, p := range strings.Split(v, ".") {
				if p == "" || strings.Trim(p, "0123456789") != "" {
					return false
				}
			}
			return true
		}}},
		"sealed-secrets":        {src: ghRel("bitnami-labs/sealed-secrets"), lines: 2, f: filter{keep: notPrefix("helm-")}},
		"selenium-node-firefox": {src: ghRel("SeleniumHQ/selenium"), n: 2, f: group(`selenium-(\d+\.\d+\.\d+)`)},
		"selenium-server":       {src: ghRel("SeleniumHQ/selenium"), n: 2, f: group(`selenium-(\d+\.\d+\.\d+)`)},
		"solr":                  {src: ghTags("apache/solr"), f: filter{keep: prefix("releases/solr/"), clean: trim("releases/solr/")}},
		"support-bundle-kit":    {src: ghRel("rancher/support-bundle-kit"), n: 1, f: filter{keep: notPrefix("0.0.")}},
		"tailscale": {src: ghRel("tailscale/tailscale"), lines: 2, f: filter{keep: func(t string) bool {
			k := vkey(t) // even minor = stable release
			return len(k) > 1 && k[1]%2 == 0
		}}},
		"trafficserver": {src: scrapeSrc("https://downloads.apache.org/trafficserver/", `trafficserver-(\d+\.\d+\.\d+)\.tar\.bz2"`), n: 1,
			f: filter{keep: func(v string) bool {
				return !strings.HasPrefix(v, "9.") && !strings.HasPrefix(v, "10.0.") && !strings.HasPrefix(v, "10.1.")
			}}},
		"tyk":     {src: ghRel("TykTechnologies/tyk"), n: 1, f: filter{keep: noDash}},
		"wildfly": {src: ghRel("wildfly/wildfly"), n: 1, f: group(`(\d+\.\d+\.\d+)\.Final`)},
		"wordpress": {src: jsonList("https://api.wordpress.org/core/stable-check/1.0/",
			func(d any) []string { m, _ := d.(map[string]any); return keys(m) }), lines: 2},
		"ztunnel": {src: ghTags("istio/ztunnel"), lines: 2, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	} {
		if _, dup := table[app]; dup {
			panic("check defined twice: " + app)
		}
		table[app] = c
	}
}

var graviteeCheck = check{src: jsonList("https://hub.docker.com/v2/repositories/graviteeio/apim-gateway/tags?page_size=100",
	func(d any) []string {
		return each(path(d, "results"), func(t any) string { return str(path(t, "name")) })
	}),
	lines: 2, f: filter{keep: full(`\d+\.\d+\.\d+`)}}

var linkerdEdge = check{src: ghRel("linkerd/linkerd2"), lines: 2, f: filter{keep: prefix("edge-"), clean: trim("edge-")}}
