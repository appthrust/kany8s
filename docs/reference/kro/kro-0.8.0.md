v0.8.1 Latest
What's Changed
fix(transform): update parent default logic for schemas with required fields by @jakobmoellerdev in #983
Full Changelog: v0.8.0...v0.8.1

Contributors
@jakobmoellerdev
jakobmoellerdev
Assets
4
kro-core-install-manifests-with-prometheus.yaml
sha256:896620d51269cadbbb24a18b40f0da6df27858a961b60cc4ac36a878c56334d3
29.1 KB
11 hours ago
kro-core-install-manifests.yaml
sha256:e8b3c66f3394c9dac49a45d12778075f2fde585a058c3c6af1ffc8180537349c
28 KB
11 hours ago
Source code
(zip)
11 hours ago
Source code
(tar.gz)
11 hours ago
v0.8.0
15 hours ago
@github-actions github-actions
 v0.8.0
 a05045e
v0.8.0
🔆 Highlights
Collections Support (KREP-002)

RGDs now support Collections: define one template and kro expands it into many resources at runtime. Collections introduce a new forEach directive that lets a CEL expression drive the iteration, and the controller keeps each generated resource in sync as items are added, changed, or removed. This makes multi‑resource expansion practical without hand‑authoring repeated blocks. (Docs, #936, #679)

Recursive Custom Types in RGD Schema

RGD schemas now allow custom types to reference other custom types, so you can build reusable, nested data structures instead of duplicating field definitions. This improves schema hygiene for complex specs and statuses while keeping validation intact. (Docs, #950)

Breaking Schema Change Detection

kro now diffs RGD schemas before updating CRDs and blocks incompatible changes (like removing fields, changing types, or adding required properties) by default. This prevents accidental breaking updates to existing instances; use kro.run/allow-breaking-changes: "true" to intentionally override. (Docs, #352)

✨ Features
feat: add Collections support + runtime/controller rewrite by @a-hilaly in #936
feat: KREP-002 declarative resource collections support by @a-hilaly in #679
feat(simpleschema): add support for recursive custom types by @shivansh-gohem in #950
feat(crd): Detect and prevent breaking schema changes in RGDs by @a-hilaly in #352
feat: add early validation for apiVersion and kind to fail fast by @antcybersec in #980
feat: add controller warmup support for faster leader failover by @a-hilaly in #955
feat: add DurationType and TimestampType conversion to go native types by @shabbskagalwala in #960
feat: add bytes conversion to go native types by @shabbskagalwala in #951
feat: add support for labels and annotations in the generated CRDs by @cnvergence in #916
🐛 Bugfixes
fix(graph): reject cluster-scoped resources with namespace set by @a-hilaly in #976
fix(schema): preserve nested array/object paths in status schema by @a-hilaly in #972
fix(schema): use date-time format for timestamps by @a-hilaly in #973
fix(dag): preserve vertex order when dependencies are satisfied by @a-hilaly in #958
fix: Convert CEL type to Go type recursively by @cirias in #940
fix: Support []object and map[string]object types in RGD schema by @kunalvirwal in #939
fix: Prevent random.* from being classified as a resource in #919
fix(release): capture GIT_VERSION once to prevent -dirty suffix in LDFLAGS by @a-hilaly in #982
fix(cluster-mgmt): ensure access to workload cluster is granted before argocd secret create by @iamahgoub in #966
fix(graph): replace panic in CRD graph builder with proper error handling by @AnshulPatil2005 in #901
⚡ Performance
chore: cache compiled CEL programs by @bschaatsbergen in #943
📖 Documentation
docs: expand collections gotchas and cross-references by @a-hilaly in #971
docs(cel): explain multiline expressions and YAML chomping by @a-hilaly in #974
fix(website): version CRD with docs snapshots by @a-hilaly in #978
Docs: manifests download URL changed to match actual URL by @hatofmonkeys in #925
fix: Fixed Quickstart instance.yaml sample on document by @ricky9408 in #913
docs: correct apiVersion for Application example by @birapjr in #899
fix: url in kubectl commands for upgrade and delete by @Fsero in #938
🧪 Testing
test: improve unit test coverage for pkg/graph/variable by @shivansh-gohem in #949
🌱 Other
refactor(instance): align node state tracking by @a-hilaly in #970
chore: bump controller-runtime to v0.23.0 and k8s deps to v0.35.0 by @a-hilaly in #956
chore: dependency bumps (go1.25.6, golangci-lint, helm, ko, chainsaw, and more) by @jakobmoellerdev in #963
chore: bump kro in kro command by @tjamet in #952
cleanup: use variadic append for enum values by @PhantomInTheWire in #910
Improve CRD cleanup skip log clarity by @skools-here in #923
Update AWS cluster management example to use EKS capabilities by @iamahgoub in #946
chore: regenerate CRDs to reflect new schema.metadata field by @a-hilaly in #977
New Contributors
@AnshulPatil2005 made their first contribution in #901
@birapjr made their first contribution in #899
@PhantomInTheWire made their first contribution in #910
@ricky9408 made their first contribution in #913
@benzaidfoued made their first contribution in #919
@skools-here made their first contribution in #923
@hatofmonkeys made their first contribution in #925
@Fsero made their first contribution in #938
@kunalvirwal made their first contribution in #939
@cirias made their first contribution in #940
@shabbskagalwala made their first contribution in #951
@shivansh-gohem made their first contribution in #949
@cnvergence made their first contribution in #916
Full Changelog: v0.7.1...v0.8.0
