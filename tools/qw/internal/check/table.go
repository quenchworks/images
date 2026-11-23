package check

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/quenchworks/images/tools/qw/internal/common"
	"github.com/quenchworks/images/tools/qw/internal/core"
)

// One row per app: a source, an optional filter, and how to report. Generated from
// scripts/check/*.py on 2026-11-13; edited by hand since.
var table = map[string]check{
	"3proxy":                            {src: ghRel("3proxy/3proxy"), n: 1, f: filter{keep: full(`[1-9]\d*\.\d+\.\d+`)}},
	"actions-runner":                    {src: ghRel("actions/runner"), n: 1},
	"activemq-artemis":                  {src: scrapeSrc("https://dlcdn.apache.org/artemis/artemis/", `href="([0-9]+\.[0-9]+\.[0-9]+)/"`), n: 1},
	"activemq":                          {src: scrapeSrc("https://downloads.apache.org/activemq/", `href="([56]\.[0-9]+\.[0-9]+)/"`), lines: 2},
	"adminer":                           {src: ghRel("vrana/adminer")},
	"akhq":                              {src: ghRel("tchiotludo/akhq"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"alertmanager":                      {src: ghRel("prometheus/alertmanager")},
	"argo-events":                       {src: ghRel("argoproj/argo-events"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"argo-rollouts":                     {src: ghRel("argoproj/argo-rollouts"), lines: 2, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"argo-workflows":                    {src: ghRel("argoproj/argo-workflows"), lines: 2},
	"artex":                             {src: ghRel("BaaRai/ARTEX"), n: 1},
	"argocd":                            {src: ghRel("argoproj/argo-cd"), lines: 2},
	"atlantis":                          {src: ghRel("runatlantis/atlantis")},
	"authelia":                          {src: ghRel("authelia/authelia")},
	"backing-image-manager":             {src: ghRel("longhorn/longhorn"), lines: 3},
	"bind":                              {src: wolfiPkg("bind"), n: 1},
	"blackbox-exporter":                 {src: ghRel("prometheus/blackbox_exporter")},
	"blocky":                            {src: ghRel("0xERR0R/blocky"), n: 1},
	"bookstack":                         {src: ghRel("BookStackApp/BookStack"), n: 1},
	"buildkit":                          {src: ghRel("moby/buildkit")},
	"buildkite-agent":                   {src: ghRel("buildkite/agent")},
	"bun":                               {src: wolfiPkg("bun")},
	"busybox":                           {src: scrapeSrc("https://busybox.net/downloads/", `busybox-(\d+\.\d+\.\d+)\.tar\.bz2`), lines: 2},
	"caddy":                             {src: ghRel("caddyserver/caddy")},
	"cadence":                           {src: ghRel("uber/cadence"), n: 1},
	"cadvisor":                          {src: ghRel("google/cadvisor")},
	"camel-k":                           {src: ghRel("apache/camel-k"), lines: 2, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"centrifugo":                        {src: ghRel("centrifugal/centrifugo"), n: 1},
	"ceph-csi-operator":                 {src: ghRel("ceph/ceph-csi-operator"), n: 1},
	"ceph":                              {src: wolfiPkg("ceph-20"), n: 1},
	"cephcsi":                           {src: ghRel("ceph/ceph-csi"), n: 1},
	"cert-manager-acmesolver":           {src: ghRel("cert-manager/cert-manager")},
	"cert-manager-cainjector":           {src: ghRel("cert-manager/cert-manager")},
	"cert-manager-controller":           {src: ghRel("cert-manager/cert-manager")},
	"cert-manager-webhook":              {src: ghRel("cert-manager/cert-manager")},
	"changedetection":                   {src: ghRel("dgtlmoon/changedetection.io"), n: 1},
	"chartmuseum":                       {src: ghRel("helm/chartmuseum")},
	"checkov":                           {src: pypiPkg("checkov"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"chisel":                            {src: ghRel("jpillora/chisel"), n: 1},
	"cilium-operator":                   {src: ghRel("cilium/cilium"), lines: 2, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"cilium":                            {src: ghRel("cilium/cilium"), lines: 2, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"cloudflared":                       {src: ghRel("cloudflare/cloudflared"), n: 1},
	"cloudnative-pg":                    {src: ghRel("cloudnative-pg/cloudnative-pg")},
	"code-server":                       {src: ghRel("coder/code-server"), n: 1},
	"composer":                          {src: wolfiPkg("composer")},
	"concourse-git-resource":            {src: ghRel("concourse/git-resource"), n: 1},
	"concourse":                         {src: ghRel("concourse/concourse"), n: 1},
	"configurable-http-proxy":           {src: npmPkg("configurable-http-proxy"), n: 1},
	"connaisseur":                       {src: ghRel("sse-secure-systems/connaisseur"), lines: 2},
	"coredns":                           {src: ghRel("coredns/coredns")},
	"coroot":                            {src: ghRel("coroot/coroot"), n: 1},
	"cosign":                            {src: ghRel("sigstore/cosign")},
	"defectdojo":                        {src: ghRel("DefectDojo/django-DefectDojo"), n: 1},
	"pairdrop":                          {src: ghRel("schlagmichdoch/PairDrop"), n: 1},
	"glances":                           {src: pypiPkg("glances"), n: 1, f: filter{keep: numDots}},
	"kestra":                            {src: ghRel("kestra-io/kestra"), n: 1},
	"commafeed":                         {src: ghRel("Athou/commafeed"), n: 1},
	"netbox":                            {src: ghRel("netbox-community/netbox"), n: 1},
	"nautobot":                          {src: pypiPkg("nautobot"), n: 1, f: filter{keep: numDots}},
	"ejbca":                             {src: ghRel("Keyfactor/ejbca-ce"), n: 1, f: filter{keep: prefix("r"), clean: trim("r")}},
	"bloodhound":                        {src: ghRel("SpecterOps/BloodHound"), n: 1},
	"nuclei":                            {src: ghRel("projectdiscovery/nuclei"), n: 1},
	"promptfoo":                         {src: npmPkg("promptfoo"), n: 1, f: filter{keep: noDash}},
	"scylla-operator":                   {src: ghRel("scylladb/scylla-operator"), n: 1},
	"strix":                             {src: ghRel("usestrix/strix"), n: 1},
	"zap":                               {src: ghLatest("zaproxy/zaproxy"), n: 1},
	"cosmian-kms":                       {src: ghRel("Cosmian/kms")},
	"crane":                             {src: wolfiPkg("crane"), n: 1},
	"cratedb":                           {src: ghRel("crate/crate"), lines: 2},
	"croc":                              {src: ghRel("schollz/croc"), n: 1},
	"crossplane":                        {src: ghRel("crossplane/crossplane"), lines: 2},
	"csi-attacher":                      {src: ghRel("kubernetes-csi/external-attacher"), lines: 3},
	"csi-livenessprobe":                 {src: ghRel("kubernetes-csi/livenessprobe"), lines: 3},
	"csi-node-driver-registrar":         {src: ghRel("kubernetes-csi/node-driver-registrar"), lines: 3},
	"csi-provisioner":                   {src: ghRel("kubernetes-csi/external-provisioner"), lines: 3},
	"csi-resizer":                       {src: ghRel("kubernetes-csi/external-resizer"), lines: 3},
	"csi-snapshotter":                   {src: ghRel("kubernetes-csi/external-snapshotter"), lines: 3},
	"cube":                              {src: ghRel("cube-js/cube"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"dagger":                            {src: ghRel("dagger/dagger"), n: 1},
	"dagster":                           {src: pypiPkg("dagster"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"dapr":                              {src: ghRel("dapr/dapr"), lines: 2},
	"datahub-frontend":                  {src: ghRel("datahub-project/datahub"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+(\.\d+)?`)}},
	"datahub-gms":                       {src: ghRel("datahub-project/datahub"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+(\.\d+)?`)}},
	"datahub-upgrade":                   {src: ghRel("datahub-project/datahub"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+(\.\d+)?`)}},
	"dependency-track-frontend":         {src: ghRel("DependencyTrack/frontend"), lines: 2},
	"dependency-track":                  {src: ghRel("DependencyTrack/dependency-track"), lines: 2},
	"dashy":                             {src: ghRel("Lissy93/dashy"), n: 1},
	"dex":                               {src: ghRel("dexidp/dex")},
	"distribution":                      {src: ghRel("distribution/distribution")},
	"dive":                              {src: ghRel("wagoodman/dive"), n: 1},
	"dnscrypt-proxy":                    {src: ghRel("DNSCrypt/dnscrypt-proxy"), n: 1},
	"dozzle":                            {src: ghRel("amir20/dozzle"), n: 1},
	"dragonfly":                         {src: ghRel("dragonflydb/dragonfly")},
	"dragonfly-operator":                {src: ghRel("dragonflydb/dragonfly-operator"), n: 1},
	"opensearch-operator":               {src: ghRel("opensearch-project/opensearch-k8s-operator"), n: 1},
	"redis-operator":                    {src: ghRel("OT-CONTAINER-KIT/redis-operator"), n: 1},
	"cass-operator":                     {src: ghRel("k8ssandra/cass-operator"), n: 1},
	"k8ssandra-operator":                {src: ghRel("k8ssandra/k8ssandra-operator"), n: 1},
	"jobset":                            {src: ghRel("kubernetes-sigs/jobset"), n: 1},
	"cdi-apiserver":                     {src: ghRel("kubevirt/containerized-data-importer"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"cdi-cloner":                        {src: ghRel("kubevirt/containerized-data-importer"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"cdi-importer":                      {src: ghRel("kubevirt/containerized-data-importer"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"cdi-controller":                    {src: ghRel("kubevirt/containerized-data-importer"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"cdi-operator":                      {src: ghRel("kubevirt/containerized-data-importer"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"cdi-uploadproxy":                   {src: ghRel("kubevirt/containerized-data-importer"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"cdi-uploadserver":                  {src: ghRel("kubevirt/containerized-data-importer"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"knative-operator":                  {src: ghRel("knative/operator"), n: 1, f: mapThen(trim("knative-v"), full(`\d+\.\d+\.\d+`))},
	"kubeflow-trainer":                  {src: ghRel("kubeflow/trainer"), n: 1},
	"volcano":                           {src: ghRel("volcano-sh/volcano"), n: 1},
	"vitess-operator":                   {src: ghRel("planetscale/vitess-operator"), n: 1},
	"duckdb":                            {src: ghRel("duckdb/duckdb")},
	"cloudreve":                         {src: ghRel("cloudreve/cloudreve"), n: 1},
	"dufs":                              {src: ghRel("sigoden/dufs"), n: 1},
	"ezbookkeeping":                     {src: ghRel("mayswind/ezbookkeeping"), n: 1},
	"adguardhome":                       {src: ghRel("AdguardTeam/AdGuardHome"), n: 1},
	"kanidm":                            {src: ghRel("kanidm/kanidm"), n: 1},
	"seerr":                             {src: ghRel("fallenbagel/jellyseerr"), n: 1},
	"semaphore":                         {src: ghRel("semaphoreui/semaphore"), n: 1},
	"sftpgo":                            {src: ghRel("drakkan/sftpgo"), n: 1},
	"easegress":                         {src: ghRel("easegress-io/easegress"), lines: 2},
	"elasticsearch":                     {src: ghRel("elastic/elasticsearch")},
	"emissary-apiext":                   {src: ghRel("emissary-ingress/emissary"), n: 1},
	"emissary":                          {src: ghRel("emissary-ingress/emissary"), n: 1},
	"emqx":                              {src: ghRel("emqx/emqx")},
	"envoy-gateway":                     {src: ghRel("envoyproxy/gateway")},
	"envoy":                             {src: ghRel("envoyproxy/envoy"), n: 1},
	"erlang":                            {wolfiPrefix: "erlang-", lines: 1, lineOnly: false},
	"ergo":                              {src: ghRel("ergochat/ergo"), n: 1},
	"rest-server":                       {src: ghRel("restic/rest-server"), n: 1},
	"restic":                            {src: ghRel("restic/restic"), n: 1},
	"hugo":                              {src: ghRel("gohugoio/hugo"), n: 3},
	"kustomize":                         {src: ghRel("kubernetes-sigs/kustomize"), n: 1, f: filter{keep: prefix("kustomize/v"), clean: trim("kustomize/v")}},
	"tflint":                            {src: ghRel("terraform-linters/tflint"), n: 1},
	"task":                              {src: ghRel("go-task/task"), n: 1},
	"microbin":                          {src: ghRel("szabodanika/microbin"), n: 1},
	"miniserve":                         {src: ghRel("svenstaro/miniserve"), n: 1},
	"lego":                              {src: ghRel("go-acme/lego"), n: 1},
	"copyparty":                         {src: ghRel("9001/copyparty"), n: 1},
	"drawio":                            {src: ghRel("jgraph/drawio"), n: 1},
	"firefly-iii":                       {src: ghRel("firefly-iii/firefly-iii"), n: 1},
	"kimai":                             {src: ghRel("kimai/kimai"), n: 1},
	"snipe-it":                          {src: ghRel("grokability/snipe-it"), n: 1},
	"leantime":                          {src: ghRel("Leantime/leantime"), n: 1},
	"koel":                              {src: ghRel("koel/koel"), n: 1},
	"lychee":                            {src: ghRel("LycheeOrg/Lychee"), n: 1},
	"shaarli":                           {src: ghRel("shaarli/Shaarli"), n: 1},
	"baikal":                            {src: ghRel("sabre-io/Baikal"), n: 1},
	"grav":                              {src: ghRel("getgrav/grav"), n: 1},
	"wallos":                            {src: ghRel("ellite/Wallos"), n: 1},
	"rss-bridge":                        {src: ghRel("RSS-Bridge/rss-bridge"), n: 1},
	"chhoto-url":                        {src: ghRel("SinTan1729/chhoto-url"), n: 1},
	"donetick":                          {src: ghRel("donetick/donetick"), n: 1},
	"apprise-api":                       {src: ghRel("caronc/apprise-api"), n: 1},
	"zipline":                           {src: ghRel("diced/zipline"), n: 1},
	"silverbullet":                      {src: ghRel("silverbulletmd/silverbullet"), n: 1},
	"flatnotes":                         {src: ghRel("dullage/flatnotes"), n: 1},
	"thelounge":                         {src: npmPkg("thelounge"), n: 1, f: filter{keep: noDash}},
	"soju":                              {src: codebergRel("emersion/soju"), n: 1},
	"go-httpbin":                        {src: ghRel("mccutchen/go-httpbin"), n: 1},
	"radicale":                          {src: ghRel("Kozea/Radicale"), n: 1},
	"cyberchef":                         {src: ghRel("gchq/CyberChef"), n: 1},
	"readeck":                           {src: codebergRel("readeck/readeck"), n: 1},
	"atuin-server":                      {src: ghRel("atuinsh/atuin"), n: 1},
	"grocy":                             {src: ghRel("grocy/grocy"), n: 1},
	"audiobookshelf":                    {src: ghRel("advplyr/audiobookshelf"), n: 1},
	"kutt":                              {src: ghRel("thedevs-network/kutt"), n: 1},
	"kener":                             {src: ghRel("rajnandan1/kener"), n: 1},
	"gotosocial":                        {src: codebergRel("superseriousbusiness/gotosocial"), n: 1},
	"wstunnel":                          {src: ghRel("erebe/wstunnel"), n: 1},
	"writefreely":                       {src: ghRel("writefreely/writefreely"), n: 1},
	"yq":                                {src: ghRel("mikefarah/yq"), n: 1},
	"age":                               {src: ghRel("FiloSottile/age"), n: 1},
	"sops":                              {src: ghRel("getsops/sops"), n: 1},
	"kubeconform":                       {src: ghRel("yannh/kubeconform"), n: 1},
	"stern":                             {src: ghRel("stern/stern"), n: 1},
	"kube-linter":                       {src: ghRel("stackrox/kube-linter"), n: 1},
	"k9s":                               {src: ghRel("derailed/k9s"), n: 1},
	"helm":                              {src: ghRel("helm/helm"), lines: 1},
	"popeye":                            {src: ghRel("derailed/popeye"), n: 1},
	"helmfile":                          {src: ghRel("helmfile/helmfile"), n: 1},
	"derper":                            {src: ghRel("tailscale/tailscale"), n: 1},
	"yopass":                            {src: ghRel("jhaals/yopass")},
	"etcd":                              {src: ghRel("etcd-io/etcd")},
	"etherpad":                          {src: ghRel("ether/etherpad"), n: 1},
	"excalidraw":                        {src: ghRel("excalidraw/excalidraw")},
	"chartdb":                           {src: ghRel("chartdb/chartdb"), n: 1},
	"psitransfer":                       {src: ghRel("psi-4ward/psitransfer"), n: 1},
	"traggo":                            {src: ghRel("traggo/server"), n: 1},
	"rallly":                            {src: ghRel("lukevella/rallly"), n: 1},
	"convertx":                          {src: ghRel("C4illin/ConvertX"), n: 1},
	"sim":                               {src: ghRel("simstudioai/sim"), n: 1},
	"sim-realtime":                      {src: ghRel("simstudioai/sim"), n: 1},
	"sim-migrations":                    {src: ghRel("simstudioai/sim"), n: 1},
	"trilium":                           {src: ghRel("TriliumNext/Trilium"), n: 1},
	"linkwarden":                        {src: ghRel("linkwarden/linkwarden"), n: 1},
	"wikijs":                            {src: ghRel("requarks/wiki"), n: 1},
	"calibre-web":                       {src: ghRel("janeczku/calibre-web"), n: 1},
	"ghostfolio":                        {src: ghRel("ghostfolio/ghostfolio"), n: 1},
	"paperless-ngx":                     {src: ghRel("paperless-ngx/paperless-ngx"), n: 1},
	"pentagi":                           {src: ghRel("vxcontrol/pentagi"), n: 1},
	"external-dns":                      {src: ghRel("kubernetes-sigs/external-dns")},
	"external-secrets":                  {src: ghRel("external-secrets/external-secrets")},
	"fabric-orderer":                    {src: ghRel("hyperledger/fabric"), lines: 2, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"fabric-peer":                       {src: ghRel("hyperledger/fabric"), lines: 2, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"falco":                             {src: ghRel("falcosecurity/falco"), n: 1},
	"ferretdb":                          {src: ghRel("FerretDB/FerretDB")},
	"filebrowser":                       {src: ghRel("filebrowser/filebrowser")},
	"flink":                             {src: scrapeSrc("https://dlcdn.apache.org/flink/", `href="flink-(\d+\.\d+\.\d+)/"`), n: 1},
	"flink-kubernetes-operator":         {src: scrapeSrc("https://dlcdn.apache.org/flink/", `href="flink-kubernetes-operator-(\d+\.\d+\.\d+)/"`), n: 1},
	"floci-full":                        {src: ghRel("floci-io/floci"), n: 1},
	"floci":                             {src: ghRel("floci-io/floci"), n: 3},
	"fluent-bit":                        {src: ghRel("fluent/fluent-bit")},
	"fluentd":                           {src: ghRel("fluent/fluentd"), lines: 2},
	"flux-helm-controller":              {src: ghRel("fluxcd/helm-controller"), n: 1},
	"flux-image-automation-controller":  {src: ghRel("fluxcd/image-automation-controller"), n: 1},
	"flux-image-reflector-controller":   {src: ghRel("fluxcd/image-reflector-controller"), n: 1},
	"flux-kustomize-controller":         {src: ghRel("fluxcd/kustomize-controller"), n: 1},
	"flux-notification-controller":      {src: ghRel("fluxcd/notification-controller"), n: 1},
	"flux-source-controller":            {src: ghRel("fluxcd/source-controller"), n: 1},
	"flux-source-watcher":               {src: ghRel("fluxcd/source-watcher"), n: 1},
	"freshrss":                          {src: ghRel("FreshRSS/FreshRSS"), n: 1},
	"frp":                               {src: ghRel("fatedier/frp"), n: 1},
	"frr":                               {src: wolfiPkg("frr-10.6"), n: 1},
	"gatus":                             {src: ghRel("TwiN/gatus"), lines: 2},
	"ghostunnel":                        {src: ghRel("ghostunnel/ghostunnel"), n: 1},
	"gitea":                             {src: ghRel("go-gitea/gitea")},
	"gitleaks":                          {src: ghRel("gitleaks/gitleaks"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"gitness":                           {src: ghRel("harness/gitness"), n: 1},
	"glance":                            {src: ghRel("glanceapp/glance"), n: 1},
	"gonic":                             {src: ghRel("sentriz/gonic"), n: 1},
	"glauth":                            {src: ghRel("glauth/glauth"), n: 1, f: filter{keep: prefix("GLAuth-v"), clean: trim("GLAuth-v")}},
	"go":                                {wolfiPrefix: "go-", lines: 2, lineOnly: false},
	"goaccess":                          {src: ghTags("allinurl/goaccess"), n: 1},
	"gokapi":                            {src: ghRel("Forceu/Gokapi"), n: 1},
	"gogs":                              {src: ghRel("gogs/gogs"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"goalert":                           {src: ghRel("target/goalert"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"gost":                              {src: ghRel("go-gost/gost"), n: 1},
	"ots":                               {src: ghRel("Luzifer/ots"), n: 1},
	"soft-serve":                        {src: ghRel("charmbracelet/soft-serve"), n: 1},
	"librespeed":                        {src: ghRel("librespeed/speedtest-go"), n: 1},
	"mox":                               {src: ghRel("mjl-/mox"), n: 1},
	"gotenberg":                         {src: ghRel("gotenberg/gotenberg"), n: 1},
	"gotify":                            {src: ghRel("gotify/server"), n: 1},
	"gradle":                            {src: wolfiPkg("gradle-9"), n: 1},
	"grafana-alloy":                     {src: ghRel("grafana/alloy"), lines: 2},
	"grafana":                           {src: ghRel("grafana/grafana")},
	"grafana-operator":                  {src: ghRel("grafana/grafana-operator"), n: 1},
	"grist":                             {src: ghRel("gristlabs/grist-core"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"grype":                             {src: ghRel("anchore/grype")},
	"hadolint":                          {src: ghRel("hadolint/hadolint"), n: 1},
	"harbor-core":                       {src: ghRel("goharbor/harbor")},
	"harbor-exporter":                   {src: ghRel("goharbor/harbor")},
	"harbor-jobservice":                 {src: ghRel("goharbor/harbor")},
	"harbor-portal":                     {src: ghRel("goharbor/harbor")},
	"harbor-registry":                   {src: ghRel("goharbor/harbor")},
	"harbor-registryctl":                {src: ghRel("goharbor/harbor")},
	"harbor-trivy-adapter":              {src: ghRel("goharbor/harbor")},
	"headscale":                         {src: ghRel("juanfont/headscale"), n: 1},
	"healthchecks":                      {src: ghRel("healthchecks/healthchecks"), n: 1},
	"homebox":                           {src: ghRel("sysadminsmedia/homebox"), n: 1},
	"homepage":                          {src: ghRel("gethomepage/homepage"), n: 1},
	"hubble-relay":                      {src: ghRel("cilium/cilium"), lines: 2, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"hubble-ui-backend":                 {src: ghRel("cilium/hubble-ui"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"hubble-ui":                         {src: ghRel("cilium/hubble-ui"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"hydra":                             {src: ghRel("ory/hydra")},
	"inbucket":                          {src: ghRel("inbucket/inbucket"), n: 1},
	"ingress-nginx":                     {wolfiPrefix: "ingress-nginx-controller-", lines: 2, lineOnly: false},
	"istio-cni":                         {src: ghRel("istio/istio"), lines: 2},
	"istio-proxyv2":                     {src: ghRel("istio/istio"), lines: 1},
	"istiod":                            {src: ghRel("istio/istio"), lines: 2},
	"jdk":                               {wolfiPrefix: "openjdk-", lines: 1, lineOnly: false},
	"jellyfin":                          {src: wolfiPkg("jellyfin"), n: 1},
	"jenkins-inbound-agent":             {src: wolfiPkg("jenkins-docker-agent-openjdk-21")},
	"jenkins":                           {src: scrapeSrc("https://get.jenkins.io/war-stable/", `href="(\d+\.\d+\.\d+)/"`), lines: 2},
	"jetty":                             {src: scrapeSrc("https://repo1.maven.org/maven2/org/eclipse/jetty/jetty-home/maven-metadata.xml", `<version>(12\.[0-9.]+)</version>`), lines: 2},
	"jre":                               {wolfiPrefix: "openjdk-", lines: 1, lineOnly: false},
	"jupyterhub":                        {src: pypiPkg("jupyterhub"), n: 2, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"k6":                                {src: ghRel("grafana/k6"), n: 1},
	"kafka-exporter":                    {src: ghRel("danielqsj/kafka_exporter"), lines: 2},
	"kafka":                             {src: scrapeSrc("https://downloads.apache.org/kafka/", `href="(\d+\.\d+\.\d+)/"`), lines: 2},
	"kanboard":                          {src: ghRel("kanboard/kanboard"), n: 1},
	"karapace":                          {src: ghRel("Aiven-Open/karapace"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"keda":                              {src: ghRel("kedacore/keda"), n: 1},
	"keycloak":                          {src: ghRel("keycloak/keycloak")},
	"kfp-api-server":                    {src: ghRel("kubeflow/pipelines"), n: 1},
	"kfp-driver":                        {src: ghRel("kubeflow/pipelines"), n: 1},
	"kfp-frontend":                      {src: ghRel("kubeflow/pipelines"), n: 1},
	"kfp-launcher":                      {src: ghRel("kubeflow/pipelines"), n: 1},
	"kfp-persistence-agent":             {src: ghRel("kubeflow/pipelines"), n: 1},
	"kfp-scheduled-workflow-controller": {src: ghRel("kubeflow/pipelines"), n: 1},
	"ko":                                {src: ghRel("ko-build/ko"), lines: 2},
	"kong":                              {src: ghTags("Kong/kong"), n: 1},
	"kopia":                             {src: ghRel("kopia/kopia"), n: 1},
	"krakend":                           {src: ghRel("krakend/krakend-ce"), n: 1},
	"kratos":                            {src: ghRel("ory/kratos")},
	"kroxylicious":                      {src: ghRel("kroxylicious/kroxylicious"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"kserve-controller":                 {src: ghRel("kserve/kserve"), n: 1},
	"kube-state-metrics":                {src: ghRel("kubernetes/kube-state-metrics")},
	"kube-vip":                          {src: ghRel("kube-vip/kube-vip"), lines: 2},
	"kubectl":                           {wolfiPrefix: "kubectl-", lines: 2, lineOnly: false},
	"kuberay":                           {src: ghRel("ray-project/kuberay")},
	"kubescape-node-agent":              {src: ghRel("kubescape/node-agent"), n: 1},
	"kubescape-operator":                {src: ghRel("kubescape/operator"), n: 1},
	"kubescape-storage":                 {src: ghRel("kubescape/storage"), n: 1},
	"kubescape-synchronizer":            {src: ghRel("kubescape/synchronizer"), n: 1},
	"kubescape":                         {src: ghRel("kubescape/kubescape"), n: 1},
	"kubevuln":                          {src: ghRel("kubescape/kubevuln"), n: 1},
	"kubo":                              {src: ghRel("ipfs/kubo"), lines: 2},
	"kueue":                             {src: ghRel("kubernetes-sigs/kueue"), n: 1},
	"kuma":                              {src: ghRel("kumahq/kuma"), n: 1},
	"kured":                             {src: ghRel("kubereboot/kured"), lines: 2},
	"kvrocks":                           {src: ghRel("apache/kvrocks"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"kyverno":                           {src: ghRel("kyverno/kyverno"), n: 1},
	"label-studio":                      {src: pypiPkg("label-studio"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"linkding":                          {src: ghRel("sissbruecker/linkding"), n: 1},
	"listmonk":                          {src: ghRel("knadh/listmonk"), n: 1},
	"litellm":                           {src: pypiPkg("litellm"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"litestream":                        {src: ghRel("benbjohnson/litestream")},
	"livekit":                           {src: ghRel("livekit/livekit"), n: 1},
	"loki":                              {src: ghRel("grafana/loki")},
	"longhorn-engine":                   {src: ghRel("longhorn/longhorn"), lines: 3},
	"longhorn-instance-manager":         {src: ghRel("longhorn/longhorn"), lines: 3},
	"longhorn-manager":                  {src: ghRel("longhorn/longhorn"), lines: 3},
	"longhorn-share-manager":            {src: ghRel("longhorn/longhorn"), lines: 3},
	"longhorn-ui":                       {src: ghRel("longhorn/longhorn"), lines: 3},
	"maddy":                             {src: ghRel("foxcpp/maddy"), n: 1},
	"mailpit":                           {src: ghRel("axllent/mailpit")},
	"mariadb":                           {src: scrapeSrc("https://archive.mariadb.org/", `mariadb-(\d+\.\d+\.\d+)/`), lines: 2},
	"mastodon-streaming":                {src: ghRel("mastodon/mastodon"), n: 1},
	"mastodon":                          {src: ghRel("mastodon/mastodon"), n: 1},
	"mattermost":                        {src: ghRel("mattermost/mattermost")},
	"maven":                             {src: wolfiPkg("maven-3.9")},
	"mealie":                            {src: ghRel("mealie-recipes/mealie"), n: 1},
	"mediamtx":                          {src: ghRel("bluenviron/mediamtx"), n: 1},
	"meilisearch":                       {src: ghRel("meilisearch/meilisearch")},
	"memos":                             {src: ghRel("usememos/memos"), n: 1},
	"miniflux":                          {src: ghRel("miniflux/v2"), n: 1},
	"mitmproxy":                         {src: pypiPkg("mitmproxy"), n: 1},
	"ml-metadata-store-server":          {src: ghRel("google/ml-metadata"), n: 1},
	"mongodb-exporter":                  {src: ghRel("percona/mongodb_exporter")},
	"mysqld-exporter":                   {src: ghRel("prometheus/mysqld_exporter")},
	"nats-jetstream-controller":         {src: ghRel("nats-io/nack"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"nats-server-config-reloader":       {src: ghRel("nats-io/nack"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"nats":                              {src: ghRel("nats-io/nats-server")},
	"navidrome":                         {src: ghRel("navidrome/navidrome"), n: 1},
	"nebula":                            {src: ghRel("slackhq/nebula"), lines: 2},
	"netdata":                           {src: ghRel("netdata/netdata"), f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"nextcloud":                         {src: ghRel("nextcloud/server"), lines: 1},
	"nginx-gateway-fabric":              {src: ghRel("nginx/nginx-gateway-fabric")},
	"nginx-proxy-manager":               {src: ghRel("NginxProxyManager/nginx-proxy-manager"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"node-exporter":                     {src: ghRel("prometheus/node_exporter")},
	"node-feature-discovery":            {src: ghRel("kubernetes-sigs/node-feature-discovery"), n: 1},
	"notation":                          {src: ghRel("notaryproject/notation"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"nsq":                               {src: ghRel("nsqio/nsq"), n: 3},
	"ntfy":                              {src: ghRel("binwiederhier/ntfy"), n: 1},
	"oauth2-proxy":                      {src: ghRel("oauth2-proxy/oauth2-proxy")},
	"odyssey":                           {src: ghRel("yandex/odyssey"), n: 1},
	"ollama":                            {src: ghRel("ollama/ollama")},
	"opa-gatekeeper":                    {src: ghRel("open-policy-agent/gatekeeper"), lines: 2, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"opa":                               {src: ghRel("open-policy-agent/opa")},
	"openbao":                           {src: ghRel("openbao/openbao")},
	"opengist":                          {src: ghRel("thomiceli/opengist"), n: 1},
	"opencost":                          {src: ghRel("opencost/opencost"), lines: 2},
	"openfga":                           {src: ghRel("openfga/openfga"), n: 1},
	"openldap":                          {wolfiPrefix: "openldap-", lines: 2, lineOnly: false},
	"openscap":                          {src: wolfiPkg("openscap"), n: 1},
	"opensearch-dashboards":             {src: ghTags("opensearch-project/OpenSearch-Dashboards"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}}, // releases lag the tags (3.10.0 tagged 2026-11-12, no release object)
	"opensearch":                        {src: ghRel("opensearch-project/OpenSearch")},
	"openssl":                           {wolfiPrefix: "openssl-", lines: 2, lineOnly: false},
	"opentelemetry-operator":            {src: ghRel("open-telemetry/opentelemetry-operator"), lines: 2, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"opentofu":                          {src: ghRel("opentofu/opentofu")},
	"openziti":                          {src: ghRel("openziti/ziti"), lines: 2, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"oras":                              {src: ghRel("oras-project/oras"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"orientdb":                          {src: scrapeSrc("https://repo1.maven.org/maven2/com/orientechnologies/orientdb-community/maven-metadata.xml", `<version>(\d+\.\d+\.\d+)</version>`), lines: 3},
	"ory-hydra":                         {src: ghRel("ory/hydra"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"otel-collector":                    {src: ghRel("open-telemetry/opentelemetry-collector-releases")},
	"owncast":                           {src: ghRel("owncast/owncast"), n: 1},
	"perl":                              {src: wolfiPkg("perl")},
	"perses":                            {src: ghRel("perses/perses"), n: 1},
	"pgadmin":                           {src: pypiPkg("pgadmin4"), n: 1, f: filter{keep: full(`\d+\.\d+`)}},
	"pgdog":                             {src: ghRel("pgdogdev/pgdog"), n: 1},
	"pgpool":                            {src: scrapeSrc("https://pgpool.github.io/download/source/", `pgpool-II-(\d+\.\d+\.\d+)\.tar\.gz`), lines: 2},
	"php":                               {wolfiPrefix: "php-", lines: 2, lineOnly: false},
	"pinniped":                          {src: ghRel("vmware/pinniped"), n: 1},
	"pocket-id":                         {src: ghRel("pocket-id/pocket-id"), n: 1},
	"plone":                             {src: pypiPkg("Plone"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"pocketbase":                        {src: ghRel("pocketbase/pocketbase"), n: 1},
	"poetry":                            {src: wolfiPkg("py3.13-poetry-bin")},
	"polaris":                           {src: ghRel("FairwindsOps/polaris"), lines: 2},
	"policy-reporter":                   {src: ghRel("kyverno/policy-reporter"), f: filter{keep: full(`v?\d+\.\d+\.\d+`)}},
	"postgres-exporter":                 {src: ghRel("prometheus-community/postgres_exporter")},
	"postgresql-15":                     {src: scrapeSrc("https://ftp.postgresql.org/pub/source/", `href="v(15\.\d+)/"`)},
	"postgresql":                        {src: scrapeSrc("https://ftp.postgresql.org/pub/source/", `href="v(\d+\.\d+(?:\.\d+)?)/"`), lines: 1},
	"spilo":                             {src: wolfiPkg("postgresql-18"), n: 1},
	"postgres-operator":                 {src: ghRel("zalando/postgres-operator"), n: 1},
	"postgrest":                         {src: ghRel("PostgREST/postgrest")},
	"prefect":                           {src: pypiPkg("prefect"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"privatebin":                        {src: ghRel("PrivateBin/PrivateBin"), n: 1},
	"prometheus-config-reloader":        {src: ghRel("prometheus-operator/prometheus-operator"), n: 1},
	"prometheus-nats-exporter":          {src: ghRel("nats-io/prometheus-nats-exporter"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"prometheus-operator":               {src: ghRel("prometheus-operator/prometheus-operator"), n: 1},
	"prometheus":                        {src: ghRel("prometheus/prometheus")},
	"promxy":                            {src: ghRel("jacksontj/promxy"), n: 1},
	"pulsar":                            {src: ghRel("apache/pulsar")},
	"pulumi":                            {src: ghRel("pulumi/pulumi")},
	"pushgateway":                       {src: ghRel("prometheus/pushgateway"), n: 1},
	"pyroscope":                         {src: ghRel("grafana/pyroscope"), n: 1},
	"python":                            {wolfiPrefix: "python-", lines: 2, lineOnly: false},
	"qdrant":                            {src: ghRel("qdrant/qdrant")},
	"questdb":                           {src: ghRel("questdb/questdb"), n: 3},
	"quickwit":                          {src: ghRel("quickwit-oss/quickwit"), n: 1},
	"rabbitmq":                          {src: ghRel("rabbitmq/rabbitmq-server")},
	"rabbitmq-cluster-operator":         {src: ghRel("rabbitmq/cluster-operator"), n: 1},
	"rclone":                            {src: ghRel("rclone/rclone")},
	"redis-exporter":                    {src: ghRel("oliver006/redis_exporter")},
	"redis":                             {src: ghRel("redis/redis")},
	"redmine":                           {src: scrapeSrc("https://www.redmine.org/releases/", `redmine-([0-9]+\.[0-9]+\.[0-9]+)\.tar\.gz`), n: 1},
	"rethinkdb":                         {src: wolfiPkg("rethinkdb"), n: 1},
	"rook":                              {src: ghRel("rook/rook"), n: 1},
	"rqlite":                            {src: ghRel("rqlite/rqlite"), n: 1},
	"ruby":                              {wolfiPrefix: "ruby-", lines: 2, lineOnly: false},
	"rust":                              {wolfiPrefix: "rust-", lines: 2, lineOnly: false},
	"shiori":                            {src: ghRel("go-shiori/shiori"), n: 1},
	"shlink":                            {src: ghRel("shlinkio/shlink"), n: 1},
	"satosa":                            {src: pypiPkg("satosa"), lines: 2, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"seaweedfs":                         {src: ghRel("seaweedfs/seaweedfs")},
	"secrets-store-csi-driver":          {src: ghRel("kubernetes-sigs/secrets-store-csi-driver"), n: 2},
	"shellcheck":                        {src: ghRel("koalaman/shellcheck"), n: 1},
	"sish":                              {src: ghRel("antoniomika/sish"), n: 1},
	"skipper":                           {src: ghRel("zalando/skipper"), n: 1},
	"skopeo":                            {src: ghRel("containers/skopeo"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"skywalking":                        {src: ghRel("apache/skywalking"), lines: 2},
	"smokescreen":                       {src: ghRel("stripe/smokescreen"), n: 1},
	"sonar-scanner-cli":                 {src: wolfiPkg("sonar-scanner-cli")},
	"sozu":                              {src: ghRel("sozu-proxy/sozu"), n: 1},
	"spark":                             {wolfiPrefix: "spark-", lines: 2, lineOnly: false},
	"spicedb":                           {src: ghRel("authzed/spicedb"), n: 1},
	"spire":                             {src: ghRel("spiffe/spire"), n: 1},
	"squid":                             {src: wolfiPkg("squid"), lines: 1},
	"step-issuer":                       {src: ghRel("smallstep/step-issuer"), f: filter{keep: full(`v?\d+\.\d+\.\d+`)}},
	"strimzi-operator":                  {src: ghRel("strimzi/strimzi-kafka-operator"), n: 1},
	"syft":                              {src: ghRel("anchore/syft")},
	"syncthing":                         {src: ghRel("syncthing/syncthing"), lines: 2},
	"tekton":                            {src: ghRel("tektoncd/pipeline"), n: 1},
	"telegraf":                          {src: ghRel("influxdata/telegraf"), lines: 2},
	"teleport":                          {src: ghRel("gravitational/teleport"), n: 1},
	"warpgate":                          {src: ghRel("warp-tech/warpgate"), n: 1},
	"netbird":                           {src: ghRel("netbirdio/netbird"), n: 1},
	"netbird-management":                {src: ghRel("netbirdio/netbird"), n: 1},
	"netbird-signal":                    {src: ghRel("netbirdio/netbird"), n: 1},
	"netbird-relay":                     {src: ghRel("netbirdio/netbird"), n: 1},
	"netbird-server":                    {src: ghRel("netbirdio/netbird"), n: 1},
	"netbird-reverse-proxy":             {src: ghRel("netbirdio/netbird"), n: 1},
	"netbird-upload":                    {src: ghRel("netbirdio/netbird"), n: 1},
	"netbird-nblink":                    {src: ghRel("netbirdio/netbird"), n: 1},
	"netbird-dashboard":                 {src: ghRel("netbirdio/dashboard"), n: 1},
	"tempo":                             {src: ghRel("grafana/tempo")},
	"temporal-cli":                      {src: ghRel("temporalio/cli")},
	"temporal":                          {src: ghRel("temporalio/temporal")},
	"temporal-ui":                       {src: ghRel("temporalio/ui-server"), n: 1},
	"terragrunt":                        {src: ghRel("gruntwork-io/terragrunt"), n: 1},
	"terralist":                         {src: ghRel("terralist/terralist"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"tetragon-operator":                 {src: ghRel("cilium/tetragon"), n: 1},
	"tetragon":                          {src: ghRel("cilium/tetragon"), n: 1},
	"thanos":                            {src: ghRel("thanos-io/thanos")},
	"tigera-operator":                   {src: ghRel("tigera/operator"), n: 1},
	"tigerbeetle":                       {src: ghRel("tigerbeetle/tigerbeetle"), n: 3},
	"tidb-operator":                     {src: ghRel("pingcap/tidb-operator"), n: 1, f: filter{keep: full(`1\.\d+\.\d+`)}}, // v1 line; v2 is a rewrite with new CRDs
	"tika":                              {src: scrapeSrc("https://archive.apache.org/dist/tika/", `href="(\d+\.\d+\.\d+)/"`), n: 1},
	"timescaledb":                       {src: ghRel("timescale/timescaledb")},
	"tinyauth":                          {src: ghRel("tinyauthapp/tinyauth"), n: 1},
	"traefik":                           {src: ghRel("traefik/traefik")},
	"trickster":                         {src: ghRel("trickstercache/trickster"), lines: 2},
	"trino":                             {src: ghRel("trinodb/trino"), n: 1},
	"trivy-operator":                    {src: ghRel("aquasecurity/trivy-operator"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"trivy-scanner":                     {src: ghRel("aquasecurity/trivy"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"trivy":                             {src: ghRel("aquasecurity/trivy")},
	"trufflehog":                        {src: ghRel("trufflesecurity/trufflehog"), lines: 2, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"trust-manager":                     {src: ghRel("cert-manager/trust-manager"), f: filter{keep: full(`v?\d+\.\d+\.\d+`)}},
	"tusd":                              {src: ghRel("tus/tusd"), n: 1},
	"typesense":                         {src: ghRel("typesense/typesense"), n: 3},
	"umami":                             {src: ghRel("umami-software/umami"), n: 1},
	"unleash":                           {src: npmPkg("unleash-server"), lines: 2, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"uptime-kuma":                       {src: ghRel("louislam/uptime-kuma"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"uv":                                {src: wolfiPkg("uv")},
	"valkey":                            {src: ghRel("valkey-io/valkey")},
	"vaultwarden":                       {src: ghRel("dani-garcia/vaultwarden"), n: 1},
	"vcluster":                          {src: ghRel("loft-sh/vcluster")},
	"vector":                            {src: ghRel("vectordotdev/vector")},
	"velero-plugin-for-aws":             {src: ghRel("vmware-tanzu/velero-plugin-for-aws"), n: 1},
	"velero":                            {src: ghRel("vmware-tanzu/velero"), n: 1},
	"versitygw":                         {src: ghRel("versity/versitygw"), lines: 2},
	"victorialogs":                      {src: ghRel("VictoriaMetrics/VictoriaLogs"), n: 1},
	"victoriametrics":                   {src: ghRel("VictoriaMetrics/VictoriaMetrics")},
	"victoriametrics-operator":          {src: ghRel("VictoriaMetrics/operator"), n: 1},
	"victoriametrics-operator-config-reloader": {src: ghRel("VictoriaMetrics/operator"), n: 1},
	"vikunja":     {src: ghRel("go-vikunja/vikunja")},
	"vitess":      {src: ghRel("vitessio/vitess"), lines: 1},
	"vouch-proxy": {src: ghTags("vouch/vouch-proxy"), n: 1},
	"wakapi":      {src: ghRel("muety/wakapi"), n: 1},
	"weaviate":    {src: ghRel("weaviate/weaviate")},
	"wiremock":    {src: scrapeSrc("https://repo1.maven.org/maven2/org/wiremock/wiremock-jetty12/maven-metadata.xml", `<version>(\d+\.\d+\.\d+)</version>`), n: 1},
	"woodpecker":  {src: ghRel("woodpecker-ci/woodpecker"), n: 1},
	"xwiki":       {src: scrapeSrc("https://maven.xwiki.org/releases/org/xwiki/platform/xwiki-platform-distribution-war/maven-metadata.xml", `<version>(\d+\.\d+\.\d+)</version>`), lines: 2},
	"xyops":       {src: ghRel("pixlcore/xyops")},
	"yarr":        {src: ghRel("nkanaev/yarr"), n: 1},
	"yourls":      {src: ghRel("YOURLS/YOURLS"), n: 1},
	"zipkin":      {src: ghRel("openzipkin/zipkin"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"komga":       {src: ghRel("gotson/komga"), n: 1},
	"zitadel":     {src: ghRel("zitadel/zitadel")},
	"zoraxy":      {src: ghRel("tobychui/zoraxy"), n: 1},
	"zot":         {src: ghRel("project-zot/zot"), n: 1},

	"actual":                          {src: npmPkg("@actual-app/sync-server"), n: 1, f: filter{keep: noDash}},
	"airflow":                         {src: pypiPkg("apache-airflow"), lines: 2, f: filter{keep: numDots}},
	"ansible":                         {src: pypiPkg("ansible"), n: 2, f: filter{keep: numDots}},
	"apache-pinot":                    {src: ghRel("apache/pinot"), n: 1, f: group(`(?:release-)?(\d+\.\d+\.\d+)`)},
	"apache-polaris":                  {src: ghRel("apache/polaris"), n: 1, f: group(`apache-polaris-(\d+\.\d+\.\d+)`)},
	"aspnet":                          dotnetCheck,
	"dotnet-runtime":                  dotnetCheck,
	"dotnet":                          dotnetCheck,
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
		func(d any) []string { return []string{common.Str(common.Path(d, "helper", "version"))} })},
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
	"flyway":                               {src: ghRel("flyway/flyway"), n: 1, f: mapThen(trim("flyway-"), full(`\d+\.\d+\.\d+`))},
	"unbound":                              {src: ghRel("NLnetLabs/unbound"), n: 1, f: mapThen(trim("release-"), full(`\d+\.\d+\.\d+`))},
	"clickhouse-operator":                  {src: ghRel("Altinity/clickhouse-operator"), n: 1, f: mapThen(trim("release-"), full(`\d+\.\d+\.\d+`))},
	"clickhouse-operator-metrics-exporter": {src: ghRel("Altinity/clickhouse-operator"), n: 1, f: mapThen(trim("release-"), full(`\d+\.\d+\.\d+`))},
	"friendica":                            {src: scrapeSrc("https://files.friendi.ca/", `friendica-full-(\d{4}\.\d{2}(?:-\d+)?)\.tar\.gz"`), n: 1},
	"garage": {src: jsonList("https://git.deuxfleurs.fr/api/v1/repos/Deuxfleurs/garage/tags?limit=50",
		func(d any) []string {
			return common.Each(d, func(t any) string { return strings.TrimLeft(common.Str(common.Path(t, "name")), "v") })
		}), lines: 2},
	"ghost":    {src: npmPkg("ghost"), f: filter{keep: noDash}},
	"renovate": {src: npmPkg("renovate"), f: filter{keep: noDash}},
	"pnpm":     {src: npmPkg("pnpm"), lines: 1, f: filter{keep: noDash}},
	"gitaly": {src: jsonList("https://gitlab.com/api/v4/projects/gitlab-org%2Fgitaly/repository/tags?per_page=40",
		func(d any) []string {
			return common.Each(d, func(r any) string {
				v := strings.TrimLeft(common.Str(common.Path(r, "name")), "v")
				if strings.Contains(v, "-") {
					return ""
				}
				return v
			})
		}), n: 1},
	"gitlab-runner": {src: jsonList("https://gitlab.com/api/v4/projects/gitlab-org%2Fgitlab-runner/releases?per_page=40",
		func(d any) []string {
			return common.Each(d, func(r any) string {
				if b, _ := common.Path(r, "upcoming_release").(bool); b {
					return ""
				}
				return strings.TrimLeft(common.Str(common.Path(r, "tag_name")), "v")
			})
		})},
	"gitlab-shell": {src: jsonList("https://gitlab.com/api/v4/projects/gitlab-org%2Fgitlab-shell/repository/tags?per_page=40",
		func(d any) []string {
			return common.Each(d, func(r any) string { return strings.TrimLeft(common.Str(common.Path(r, "name")), "v") })
		}), n: 1},
	"gitlab-rails":              gitlabFossCheck,
	"gitlab-workhorse":          gitlabFossCheck,
	"gravitee-console":          graviteeCheck,
	"gravitee-gateway":          graviteeCheck,
	"gravitee-management-api":   graviteeCheck,
	"guacamole":                 {src: ghTags("apache/guacamole-client"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"guacd":                     {src: ghTags("apache/guacamole-server"), n: 1, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"hasura":                    {src: ghRel("hasura/graphql-engine"), lines: 2, f: filter{keep: prefix("2.")}},
	"influxdb":                  {src: ghRel("influxdata/influxdb"), f: filter{keep: prefix("2.")}},
	"jaeger":                    {src: ghRel("jaegertracing/jaeger"), n: 1, f: filter{keep: prefix("2.")}},
	"mercure":                   {src: ghRel("dunglas/mercure"), n: 1},
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
		func(d any) []string { return []string{common.Str(common.Path(d, "dist-tags", "latest"))} }), n: 1},
	"mysql":  {src: jsonList("https://endoflife.date/api/mysql.json", endoflifeLatest), lines: 3}, // 8.4 LTS, 9.7 LTS, newest Innovation (#128)
	"neo4j":  {src: ghTags("neo4j/neo4j"), lines: 2, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	"nessie": {src: ghRel("projectnessie/nessie"), n: 1, f: group(`nessie-(\d+\.\d+\.\d+)`)},
	"nifi":   {src: ghTags("apache/nifi"), n: 1, f: group(`rel/nifi-(\d+\.\d+\.\d+)`)},
	"odoo": {src: func() ([]string, error) {
		ds, err := core.Scrape("https://nightly.odoo.com/19.0/nightly/src/", `odoo_19\.0\.([0-9]{8})\.tar\.gz`)
		for i := range ds {
			ds[i] = "19.0." + ds[i]
		}
		return ds, err
	}, n: 1},
	"openresty": {src: ghTags("openresty/openresty"), lines: 2, f: filter{keep: full(`\d+\.\d+\.\d+\.\d+`)}},
	"pgbouncer": {src: ghRel("pgbouncer/pgbouncer"), f: filter{clean: func(t string) string {
		return strings.ReplaceAll(strings.ReplaceAll(t, "pgbouncer_", ""), "_", ".")
	}}},
	"pgadmin4": {src: ghTags("pgadmin-org/pgadmin4"), n: 1, f: filter{keep: full(`REL-\d+_\d+`), clean: func(t string) string {
		return strings.ReplaceAll(strings.TrimPrefix(t, "REL-"), "_", ".")
	}}},
	"phpmyadmin": {src: jsonList("https://www.phpmyadmin.net/home_page/version.json",
		func(d any) []string { return []string{common.Str(common.Path(d, "version"))} }), n: 1},
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
		k := core.Vkey(t) // even minor = stable release
		return len(k) > 1 && k[1]%2 == 0
	}}},
	"trafficserver": {src: scrapeSrc("https://downloads.apache.org/trafficserver/", `trafficserver-(\d+\.\d+\.\d+)\.tar\.bz2"`), n: 1,
		f: filter{keep: func(v string) bool {
			return !strings.HasPrefix(v, "9.") && !strings.HasPrefix(v, "10.0.") && !strings.HasPrefix(v, "10.1.")
		}}},
	"tyk":     {src: ghRel("TykTechnologies/tyk"), n: 1, f: filter{keep: noDash}},
	"wildfly": {src: ghRel("wildfly/wildfly"), n: 1, f: group(`(\d+\.\d+\.\d+)\.Final`)},
	"wordpress": {src: jsonList("https://api.wordpress.org/core/stable-check/1.0/",
		func(d any) []string { m, _ := d.(map[string]any); return common.Keys(m) }), lines: 2},
	"ztunnel": {src: ghTags("istio/ztunnel"), lines: 2, f: filter{keep: full(`\d+\.\d+\.\d+`)}},

	"proxysql": {run: func(w io.Writer) error {
		vs, err := core.Wolfi("proxysql")
		if err != nil {
			return err
		}
		reportLines(w, "proxysql", vs, 1, filter{})
		up, err := core.Github("sysown/proxysql")
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "  upstream (github.com/sysown/proxysql): %s\n", strings.Join(up[:min(3, len(up))], ", "))
		return nil
	}},
	"beszel": {src: ghRel("henrygd/beszel"), n: 1},
	"bifrost": {src: ghTags("maximhq/bifrost"), lines: 2, f: filter{
		keep:  func(t string) bool { return strings.HasPrefix(t, "transports/v") && !strings.Contains(t, "-") },
		clean: func(t string) string { return strings.TrimLeft(strings.SplitN(t, "/", 2)[1], "v") }}},
	"cassandra": {src: ghTags("apache/cassandra"), f: filter{keep: full(`cassandra-\d+\.\d+\.\d+`), clean: trim("cassandra-")}},
	"scylladb":  {src: ghTags("scylladb/scylladb"), f: filter{keep: full(`scylla-\d+\.\d+\.\d+`), clean: trim("scylla-")}},
	"zookeeper": {src: ghTags("apache/zookeeper"), f: filter{keep: full(`release-\d+\.\d+\.\d+`), clean: trim("release-")}},
	"cluster-autoscaler": {src: ghRel("kubernetes/autoscaler"), lines: 2, f: filter{
		keep: regexp.MustCompile(`^cluster-autoscaler-\d`).MatchString, clean: trim("cluster-autoscaler-")}},
	// deno is frozen at 2.9.7 until its hold clears (apps/deno/build.conf).
	"deno": {src: ghRel("denoland/deno"), f: filter{keep: func(t string) bool {
		for _, p := range strings.Split(t, ".") {
			if p == "" || strings.Trim(p, "0123456789") != "" {
				return false
			}
		}
		return !core.Vless("2.9.7", t)
	}}},
	// RE2 has no lookahead: "not followed by a dot" (skips the .asc/.sha files) as a class.
	"httpd": {src: scrapeSrc("https://downloads.apache.org/httpd/", `httpd-(2\.4\.\d+)\.tar\.bz2(?:[^.]|$)`), n: 1},
	"mongodb": {src: jsonList("https://downloads.mongodb.org/current.json", func(d any) []string {
		return common.Each(common.Path(d, "versions"), func(v any) string { return common.Str(common.Path(v, "version")) })
	}), f: filter{keep: full(`\d+\.0\.\d+`)}},
	"powerdns":          pdnsRefs("auth-"),
	"powerdns-recursor": pdnsRefs("rec-"),
	"storm":             {src: scrapeSrc("https://archive.apache.org/dist/storm/", `href="apache-storm-(\d+\.\d+\.\d+)/"`), n: 1},
	"tomcat":            {src: scrapeSrc("https://downloads.apache.org/tomcat/tomcat-11/", `href="v(11\.0\.\d+)/"`), n: 1},
	"tomee":             {src: scrapeSrc("https://archive.apache.org/dist/tomee/", `href="tomee-(\d+\.\d+\.\d+)/"`), n: 1},
	// github() strips the leading "v" of every tag, so these tags read "ertical-pod-autoscaler-".
	"vertical-pod-autoscaler": {src: ghRel("kubernetes/autoscaler"), lines: 2, f: filter{
		keep: func(t string) bool {
			return strings.HasPrefix(t, "ertical-pod-autoscaler-") && !strings.HasPrefix(t, "ertical-pod-autoscaler-chart")
		},
		clean: trim("ertical-pod-autoscaler-")}},
	"static": {run: func(w io.Writer) error {
		ver := "datestamped"
		if b, err := os.ReadFile(filepath.Join(core.ImagesRoot, "apps", "static", "build.conf")); err == nil {
			if m := regexp.MustCompile(`VERSIONS=\(([^)]*)\)`).FindStringSubmatch(string(b)); m != nil {
				if f := strings.Fields(m[1]); len(f) > 0 {
					ver = f[len(f)-1]
				}
			}
		}
		fmt.Fprintf(w, "static               have=%13s  latest=%13s  ok  (datestamped base, no upstream to track)\n", ver, ver)
		return nil
	}},
	"drupal": {src: scrapeSrc("https://updates.drupal.org/release-history/drupal/current", `<version>(\d+\.\d+\.\d+)</version>`), lines: 2},
	"nginx": {src: ghTags("nginx/nginx"), f: filter{keep: func(t string) bool {
		m := regexp.MustCompile(`^release-\d+\.(\d+)\.\d+$`).FindStringSubmatch(t)
		return m != nil && core.Vkey(m[1])[0]%2 == 0 // even minor = stable
	}, clean: trim("release-")}},

	"apisix-ingress-controller": {src: scrapeSrc("https://archive.apache.org/dist/apisix/ingress-controller/", `href="(\d+\.\d+\.\d+)/"`), n: 1},
	// the newest four 1.x lines, each listing page scraped for its tarballs
	"mediawiki": {src: func() ([]string, error) {
		const base = "https://releases.wikimedia.org/mediawiki/"
		ls, err := core.Scrape(base, `href="(1\.\d+)/"`)
		if err != nil {
			return nil, err
		}
		ls = uniq(ls)
		core.SortDesc(ls)
		ls = ls[:min(4, len(ls))]
		var out []string
		for _, l := range ls {
			vs, err := core.Scrape(base+l+"/", `mediawiki-(\d+\.\d+\.\d+)\.tar\.gz"`)
			if err != nil {
				return nil, err
			}
			out = append(out, vs...)
		}
		return out, nil
	}, lines: 2},
	"n8n":         {src: n8nStable, lines: 2},
	"n8n-runners": {src: n8nStable, lines: 2},
	"vault":       {src: ghRel("hashicorp/vault"), lines: 2, f: filter{keep: numDots}},
	"yarn": {src: ghRel("yarnpkg/berry"), f: filter{keep: prefix("@yarnpkg/cli/"),
		clean: func(t string) string { return t[strings.LastIndex(t, "/")+1:] }}},
	"zabbix-server": {src: zabbixSources, lines: 2},
	"zabbix-web":    {src: zabbixSources, lines: 2},
	"graylog":       {src: ghTags("Graylog2/graylog2-server"), lines: 2, f: filter{keep: full(`\d+\.\d+\.\d+`)}},
	// coolify pins its realtime image in versions.json; read it at the newest 4.0-4.3 tag that has one.
	// cilium-envoy follows the Envoy release the newest cilium release pins (envoy.image.tag
	// v<envoy>-<timestamp>-<cilium/proxy commit> in install/kubernetes/cilium/values.yaml).
	"cilium-envoy": {src: func() ([]string, error) {
		tags, err := core.Github("cilium/cilium")
		if err != nil || len(tags) == 0 {
			return nil, err
		}
		core.SortDesc(tags)
		return core.Scrape("https://raw.githubusercontent.com/cilium/cilium/v"+strings.TrimPrefix(tags[0], "v")+"/install/kubernetes/cilium/values.yaml",
			`tag: "v(\d+\.\d+\.\d+)-\d+-[0-9a-f]{40}"`)
	}, n: 1},
	"coolify-realtime": {src: func() ([]string, error) {
		tags, err := core.Github("coollabsio/coolify")
		if err != nil {
			return nil, err
		}
		for _, t := range tags {
			p := strings.SplitN(t, ".", 3)
			// Python compared the first two fields as STRINGS: ["4","10"] < ["4","4"]. Kept as is.
			if len(p) < 2 || !(p[0] < "4" || (p[0] == "4" && p[1] < "4")) {
				continue
			}
			var vj map[string]any
			if core.JsonGet("https://raw.githubusercontent.com/coollabsio/coolify/v"+t+"/versions.json", &vj) != nil {
				continue
			}
			for _, o := range vj {
				if v := common.Str(common.Path(o, "realtime", "version")); v != "" {
					return []string{v}, nil
				}
			}
		}
		return nil, nil
	}, n: 1},

	"authentik": {src: ghRel("goauthentik/authentik"), lines: 2, f: filter{
		keep:  func(t string) bool { return strings.Contains(t, "/") },
		clean: func(t string) string { return t[strings.LastIndex(t, "/")+1:] }}},
	// Codeberg's API first; its git refs when the API is down.
	"forgejo": {src: func() ([]string, error) {
		var rels []map[string]any
		if err := core.JsonGet("https://codeberg.org/api/v1/repos/forgejo/forgejo/releases?limit=50", &rels); err == nil {
			var tags []string
			for _, r := range rels {
				if p, _ := r["prerelease"].(bool); p {
					continue
				}
				if d, _ := r["draft"].(bool); d {
					continue
				}
				tags = append(tags, strings.TrimLeft(common.Str(r["tag_name"]), "v"))
			}
			return tags, nil
		}
		out, err := exec.Command("git", "ls-remote", "--tags", "https://codeberg.org/forgejo/forgejo.git").Output()
		if err != nil {
			return nil, err
		}
		var tags []string
		for _, m := range regexp.MustCompile(`(?m)refs/tags/v(\d+\.\d+\.\d+)$`).FindAllStringSubmatch(string(out), -1) {
			tags = append(tags, m[1])
		}
		return uniq(tags), nil
	}, lines: 2},
	"varnish": {run: func(w io.Writer) error {
		vs, err := core.Wolfi("varnish")
		if err != nil {
			return err
		}
		reportLines(w, "varnish", vs, 2, filter{})
		tags, err := core.Github("varnish/varnish")
		if err != nil {
			return err
		}
		up := map[string]string{}
		for _, t := range tags {
			v := regexp.MustCompile(`^v?arnish-`).ReplaceAllString(t, "")
			if !core.StartsDigit.MatchString(v) {
				continue
			}
			l := core.LineKey(v, 2)
			if old, ok := up[l]; !ok || core.Vless(old, v) {
				up[l] = v
			}
		}
		ls := common.Keys(up)
		core.SortDesc(ls)
		var parts []string
		for _, l := range ls[:min(4, len(ls))] {
			parts = append(parts, l+"->"+up[l])
		}
		fmt.Fprintf(w, "  upstream (github.com/varnish/varnish): %s\n", strings.Join(parts, ", "))
		return nil
	}},
	// the Kafka version marked default in the newest operator release's kafka-versions.yaml
	"strimzi-kafka": {src: func() ([]string, error) {
		tags, err := core.Github("strimzi/strimzi-kafka-operator")
		if err != nil {
			return nil, err
		}
		var ts []string
		for _, t := range tags {
			if core.StartsDigit.MatchString(t) {
				ts = append(ts, t)
			}
		}
		if len(ts) == 0 {
			return nil, nil
		}
		core.SortDesc(ts)
		newest := ts[0]
		b, err := core.Fetch("https://raw.githubusercontent.com/strimzi/strimzi-kafka-operator/" + newest + "/kafka-versions.yaml")
		if err != nil {
			return nil, err
		}
		var out []string
		for _, block := range strings.Split(string(b), "\n- ") {
			m := regexp.MustCompile(`version:\s*(\S+)`).FindStringSubmatch(block)
			if m != nil && regexp.MustCompile(`(?m)^\s*default:\s*true`).MatchString(block) {
				out = append(out, newest+"-kafka-"+m[1])
			}
		}
		return out, nil
	}, n: 1},
	// kgateway-envoy moves in lockstep with kgateway: same VERSIONS and ENVOY_INDEX.
	"kgateway-envoy": {run: func(w io.Writer) error {
		tags, err := core.Github("kgateway-dev/kgateway")
		if err != nil {
			return err
		}
		report(w, "kgateway-envoy", tags, 0, filter{})
		window := func(app string) string {
			out, _ := exec.Command("bash", "-c", "source "+core.ImagesRoot+"/apps/"+app+"/build.conf; echo ${VERSIONS[@]} '|' ${ENVOY_INDEX[@]}").Output()
			return strings.Join(strings.Fields(string(out)), " ")
		}
		cp, dp := window("kgateway"), window("kgateway-envoy")
		if cp != dp {
			fmt.Fprintf(w, "  LOCKSTEP BROKEN:    apps/kgateway %s\n                      apps/kgateway-envoy %s\n", common.PyList(strings.Fields(cp)), common.PyList(strings.Fields(dp)))
			return nil
		}
		fmt.Fprintf(w, "  lockstep:           matches apps/kgateway (%s)\n", strings.Join(core.Current("kgateway"), " "))
		return nil
	}},
	// CockroachDB has no release feed we trust: probe the binary URL for nearby versions.
	"cockroachdb": {src: func() ([]string, error) {
		cands := map[string]bool{}
		for _, v := range core.Current("cockroachdb") {
			k := core.Vkey(v)
			y, m, p := k[0], k[1], k[2]
			for q := p; q < p+6; q++ {
				cands[fmt.Sprintf("%d.%d.%d", y, m, q)] = true
			}
			for _, d := range []int{1, 2} {
				for q := 0; q < 3; q++ {
					cands[fmt.Sprintf("%d.%d.%d", y, m+d, q)] = true
				}
			}
			for q := 0; q < 3; q++ {
				cands[fmt.Sprintf("%d.1.%d", y+1, q)] = true // YY.1 opens each year
			}
		}
		hc := &http.Client{Timeout: 20 * time.Second}
		var out []string
		for v := range cands {
			r, err := hc.Head("https://binaries.cockroachdb.com/cockroach-v" + v + ".linux-amd64.tgz")
			if err == nil {
				r.Body.Close()
				if r.StatusCode == 200 {
					out = append(out, v)
				}
			}
		}
		sort.Strings(out)
		return out, nil
	}},

	// Wolfi ships elixir per Erlang major (erl28-elixir-1.19); compare per elixir line.
	"elixir": {run: func(w io.Writer) error {
		m, err := core.WolfiMatch(`^erl\d+-elixir-\d+\.\d+$`)
		if err != nil {
			return err
		}
		lines := map[string]string{}
		for _, v := range m {
			l := core.LineKey(v, 2)
			if old, ok := lines[l]; !ok || core.Vless(old, v) {
				lines[l] = v
			}
		}
		fmt.Fprintln(w, "elixir")
		var behind []string
		for _, c := range core.Current("elixir") {
			l := core.LineKey(c, 2)
			v, ok := lines[l]
			upd := ok && core.Vless(c, v)
			if upd {
				behind = append(behind, v)
			}
			if !ok {
				v = "?"
			}
			fmt.Fprintf(w, "  elixir-%s: have=%10s  latest=%10s  %s\n", l, c, v, okOr(upd))
		}
		fmt.Fprintf(w, "  => %s\n", updOr(behind))
		return nil
	}},
	// 22 is built from the nodejs.org tarball (Wolfi moved nodejs-22 to enterprise,
	// 2026-08-14); 24 and 26 come from Wolfi.
	"node": {run: func(w io.Writer) error {
		cur := map[string]string{}
		for _, v := range core.Current("node") {
			cur[strings.SplitN(v, ".", 2)[0]] = v
		}
		fmt.Fprintln(w, "node")
		var behind []string
		for _, maj := range []string{"22", "24", "26"} {
			var cands []string
			var err error
			if maj == "22" {
				var idx []map[string]any
				err = core.JsonGet("https://nodejs.org/dist/index.json", &idx)
				for _, r := range idx {
					if v := common.Str(r["version"]); strings.HasPrefix(v, "v22.") {
						cands = append(cands, v[1:])
					}
				}
			} else {
				cands, err = core.Wolfi("nodejs-" + maj)
			}
			if err != nil {
				return err
			}
			core.SortDesc(cands)
			newest := cands[0]
			have, ok := cur[maj]
			if !ok {
				have = "?"
			}
			upd := !ok || core.Vless(have, newest)
			if upd {
				behind = append(behind, newest)
			}
			fmt.Fprintf(w, "  nodejs-%s: have=%10s  latest=%10s  %s\n", maj, have, newest, okOr(upd))
		}
		fmt.Fprintf(w, "  => %s\n", updOr(behind))
		return nil
	}},
	// step-ca ships the step CLI too, pinned separately in STEP.
	"step-ca": {run: func(w io.Writer) error {
		tags, err := core.Github("smallstep/certificates")
		if err != nil {
			return err
		}
		report(w, "step-ca", tags, 1, filter{})
		out, _ := exec.Command("bash", "-c", "source "+core.ImagesRoot+"/apps/step-ca/build.conf; echo ${STEP[@]}").Output()
		step := strings.Fields(string(out))
		cli, err := core.Github("smallstep/cli")
		if err != nil {
			return err
		}
		var cands []string
		for _, t := range cli {
			if core.StartsDigit.MatchString(t) {
				cands = append(cands, t)
			}
		}
		core.SortDesc(cands)
		have, newest := "", ""
		if len(step) > 0 {
			core.SortDesc(step)
			have = step[0]
		}
		if len(cands) > 0 {
			newest = cands[0]
		}
		var status string
		switch {
		case newest == "":
			status = "BROKEN CHECK -- smallstep/cli returned NO candidates"
		case have == "":
			status = "BROKEN CHECK -- no STEP pin found in build.conf"
		case core.Vless(have, newest):
			status = "UPDATE -> ['" + newest + "']"
		default:
			status = "ok"
		}
		h, l := have, "[]"
		if h == "" {
			h = "?"
		}
		if newest != "" {
			l = common.PyList([]string{newest})
		}
		fmt.Fprintf(w, "%-20s have=%14s  latest1=%s  %s\n", "step-ca (step cli)", h, l, status)
		return nil
	}},
	// NGF pins its nginx, njs and agent per release: shown, not bumped on their own.
	"nginx-gateway-fabric-nginx": {run: func(w io.Writer) error {
		tags, err := core.Github("nginx/nginx-gateway-fabric")
		if err != nil {
			return err
		}
		const app = "nginx-gateway-fabric-nginx"
		report(w, app, tags, 0, filter{})
		ver := "?"
		if cur := core.Current(app); len(cur) > 0 {
			ver = cur[len(cur)-1]
		}
		b, _ := os.ReadFile(filepath.Join(core.ImagesRoot, "apps", app, "build.conf"))
		pinned := func(name string) string {
			m := regexp.MustCompile(`declare -A ` + name + `=\(\s*\[` + regexp.QuoteMeta(ver) + `\]=(\S+)`).FindStringSubmatch(string(b))
			if m == nil {
				return "?"
			}
			return m[1]
		}
		newest := func(cands []string) string {
			best := "?"
			for _, c := range cands {
				if full(`\d+\.\d+\.\d+`)(c) && (best == "?" || core.Vless(best, c)) {
					best = c
				}
			}
			return best
		}
		nginx, err := core.GithubTags("nginx/nginx")
		if err != nil {
			return err
		}
		for i := range nginx {
			nginx[i] = strings.ReplaceAll(nginx[i], "release-", "")
		}
		njs, err := core.GithubTags("nginx/njs")
		if err != nil {
			return err
		}
		agent, err := core.Github("nginx/agent")
		if err != nil {
			return err
		}
		var agent3 []string
		for _, t := range agent {
			if strings.HasPrefix(t, "3.") {
				agent3 = append(agent3, t)
			}
		}
		fmt.Fprintf(w, "  components pinned by NGF %s (bump only with the release):\n", ver)
		for _, r := range [][3]string{
			{"nginx", pinned("NGINX_VER"), newest(nginx)},
			{"njs", pinned("NJS_VER"), newest(njs)},
			{"nginx-agent", pinned("AGENT_VER"), newest(agent3)},
		} {
			state := "ok"
			if r[1] != r[2] {
				state = "upstream moved"
			}
			fmt.Fprintf(w, "    %-12s have=%10s  upstream-newest=%10s  %s\n", r[0], r[1], r[2], state)
		}
		return nil
	}},

	"linkerd-proxy": {run: linkerdProxy},
	"adc":           {run: adcCheck},
	"contour":       {run: envoyPaired("contour", "projectcontour/contour", contourEnvoyPin)},
	"kgateway":      {run: envoyPaired("kgateway", "kgateway-dev/kgateway", kgatewayEnvoyPin)},
	"apisix":        {run: apisixCheck},
}

// gitlabFossCheck reads the gitlab-foss release tags (workhorse and rails ship from the same tree).
var gitlabFossCheck = check{src: jsonList("https://gitlab.com/api/v4/projects/gitlab-org%2Fgitlab-foss/repository/tags?per_page=40",
	func(d any) []string {
		return common.Each(d, func(r any) string {
			v := strings.TrimLeft(common.Str(common.Path(r, "name")), "v")
			if strings.Contains(v, "-") { // rc and ee tags
				return ""
			}
			return v
		})
	}), n: 1}

// dotnetCheck reads Microsoft's .NET 10 release metadata: the images package the official tarballs (Wolfi moved
// dotnet-10 to enterprise-packages on 2026-11-12) and tag them on the SDK 1xx band (10.0.1xx).
var dotnetCheck = check{src: jsonList("https://builds.dotnet.microsoft.com/dotnet/release-metadata/10.0/releases.json",
	func(d any) []string {
		var out []string
		rels, _ := common.Path(d, "releases").([]any)
		for _, r := range rels {
			out = append(out, common.Each(common.Path(r, "sdks"), func(s any) string {
				if v := common.Str(common.Path(s, "version")); regexp.MustCompile(`^10\.0\.1\d\d$`).MatchString(v) {
					return v
				}
				return ""
			})...)
		}
		return out
	}), n: 1}
