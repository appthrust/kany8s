Title: Collections | kro

URL Source: https://kro.run/docs/concepts/rgd/resource-definitions/collections/

Markdown Content:
By default, each resource definition (in `spec.resources`) creates exactly one Kubernetes resource. This means the number of resources is fixed at design time - if you need 5 worker Pods, you write 5 resource definitions.

Collections let you declaratively manage multiple similar resources from a single definition. This is useful when the number of resources depends on runtime data like availability zones, tenants, or worker counts.

kro provides the `forEach` field to turn any resource into a collection. The field takes one or more iterators, and kro creates one resource for each element (or combination of elements), keeping them in sync as the collection changes.

This RGD uses `forEach` to create multiple Pods from a single resource entry:

```
apiVersion: kro.run/v1alpha1
kind: ResourceGraphDefinition
metadata:
  name: worker-pool
spec:
  schema:
    apiVersion: v1alpha1
    kind: WorkerPool
    spec:
      workers: "[]string"
      image: string

  resources:
    - id: workerPods
      forEach:
        - worker: ${schema.spec.workers}
      template:
        apiVersion: v1
        kind: Pod
        metadata:
          name: ${schema.metadata.name + '-' + worker}
        spec:
          containers:
            - name: app
              image: ${schema.spec.image}
```

When a user creates an instance with `workers: ["alice", "bob"]`, kro creates two Pods - one for each worker. If the user later updates the list to `["alice", "bob", "charlie"]`, kro creates a third Pod for "charlie".

The `forEach` field is an array of iterators. Each iterator is a single-entry map binding a variable name to an expression:

```
forEach:
  - region: ${schema.spec.regions}
```

Each map entry must have exactly one key-value pair:

*   **Key**: The iterator variable name (available in your template expressions)
*   **Value**: A CEL expression that **must evaluate to an array**

Arrays maintain their order, so resources are indexed in the same order as elements appear in the source array - this ordering is deterministic and predictable for naming and labeling purposes.

### Iterating Over Maps[​](https://kro.run/docs/concepts/rgd/resource-definitions/collections/#iterating-over-maps "Direct link to Iterating Over Maps")

If you have map (object) data, convert it to a sorted array first. Maps have non-deterministic iteration order in Go, so converting to a sorted array ensures consistent resource creation order:

```
# Given schema.spec.labels = {"app": "web", "env": "prod", "team": "platform"}
forEach:
  # Convert map keys to a sorted array
  - key: ${schema.spec.labels.map(k, k).sort()}
```

For ordering pitfalls and why this matters, see [Map Gotchas and pitfalls](https://kro.run/docs/concepts/rgd/resource-definitions/collections/#map-iteration-order-is-not-deterministic)

The variable name you specify becomes available in the template, holding the current element:

```
- id: workerPods
  forEach:
    - worker: ${schema.spec.workers}  # ["alice", "bob", "charlie"]
  template:
    kind: Pod
    metadata:
      # <instance-name>-alice, <instance-name>-bob, <instance-name>-charlie
      name: ${schema.metadata.name + '-' + worker}
```

To access the index, iterate over a range instead:

```
- id: workerPods
  forEach:
    - idx: ${lists.range(size(schema.spec.workers))}
  template:
    kind: Pod
    metadata:
      # <instance-name>-0, <instance-name>-1, <instance-name>-2
      name: ${schema.metadata.name + '-' + string(idx)}
    spec:
      containers:
        - name: worker
          env:
            - name: WORKER_NAME
              value: ${schema.spec.workers[idx]}  # alice, bob, charlie
```

tip

Use descriptive iterator names that reflect what you're iterating over, like `worker`, `region`, or `dbSpec` rather than generic names like `item` or `i`.

### Resource Naming[​](https://kro.run/docs/concepts/rgd/resource-definitions/collections/#resource-naming "Direct link to Resource Naming")

Each resource in a collection must have a unique name. Always include the iterator variable in `metadata.name` to ensure uniqueness:

```
- id: workerPods
  forEach:
    - worker: ${schema.spec.workers}
  template:
    kind: Pod
    metadata:
      # Good: includes iterator variable for uniqueness
      name: ${schema.metadata.name + '-' + worker}
```

Unique Names Required

Resource names must be unique not just within the collection, but across all instances and RGDs in the same namespace. Kubernetes identifies resources by namespace + name + kind + apiVersion - if two resources share these, they conflict.

Best practice: always include the instance name to avoid cross-instance collisions:

```
# With regions: ["us-east", "us-west"] and tiers: ["web", "api"]
name: ${schema.metadata.name + '-' + region + '-' + tier}
# Creates: myapp-us-east-web, myapp-us-east-api, myapp-us-west-web, myapp-us-west-api
```

If you omit `schema.metadata.name`, two instances with the same iterator values will overwrite each other's resources.

When you specify multiple iterators, kro creates resources for every combination (cartesian product) of values:

```
- id: deployments
  forEach:
    - region: ${schema.spec.regions}  # ["us-east", "us-west"]
    - tier: ${schema.spec.tiers}      # ["web", "api"]
  template:
    kind: Deployment
    metadata:
      # Creates 4 deployments: myapp-us-east-web, myapp-us-east-api, etc.
      name: ${schema.metadata.name + '-' + region + '-' + tier}
    spec:
      template:
        spec:
          containers:
            - name: app
              image: ${schema.spec.image}
              env:
                - name: REGION
                  value: ${region}
                - name: TIER
                  value: ${tier}
```

With `regions: ["us-east", "us-west"]` and `tiers: ["web", "api"]`, this creates 4 deployments covering all combinations.

### Combination Order[​](https://kro.run/docs/concepts/rgd/resource-definitions/collections/#combination-order "Direct link to Combination Order")

The first iterator is the outer loop, subsequent iterators are nested inside. Resources are created in this order:

`# forEach:#   - region: ["us-east", "us-west"]#   - tier: ["web", "api"]# Creates resources in order:# 1. region=us-east, tier=web# 2. region=us-east, tier=api# 3. region=us-west, tier=web# 4. region=us-west, tier=api`

This order is deterministic as long as each iterator's source array has deterministic order (arrays are ordered, maps are not - see [Iterating Over Maps](https://kro.run/docs/concepts/rgd/resource-definitions/collections/#iterating-over-maps) if you need to iterate over map data).

important

If any iterator's collection is empty, zero resources are created. This follows standard cartesian product behavior: `2 × 0 = 0`.

The iterator expression must evaluate to an array. Arrays can come from various sources:

*   Instance Spec
*   Resource Spec
*   Resource Status
*   Array Literal

```
spec:
  schema:
    spec:
      regions: "[]string"
  resources:
    - id: regionalConfigs
      forEach:
        - region: ${schema.spec.regions}
      template:
        kind: ConfigMap
        metadata:
          name: ${schema.metadata.name + '-config-' + region}
```

Other resources can reference a collection by its ID. The collection exposes all created resources as an array, which you can use with CEL functions like `map()`, `filter()`, and `all()`.

### In a Resource[​](https://kro.run/docs/concepts/rgd/resource-definitions/collections/#in-a-resource "Direct link to In a Resource")

Use CEL functions to aggregate collection data into a single resource:

```
- id: workerPods
  forEach:
    - worker: ${schema.spec.workers}
  template:
    kind: Pod
    metadata:
      name: ${schema.metadata.name + '-' + worker}
    # ...

- id: summary
  template:
    kind: ConfigMap
    metadata:
      name: ${schema.metadata.name + '-summary'}
    data:
      podNames: ${workerPods.map(p, p.metadata.name).join(', ')}
      count: ${string(size(workerPods))}
```

### In Another Collection[​](https://kro.run/docs/concepts/rgd/resource-definitions/collections/#in-another-collection "Direct link to In Another Collection")

One collection can iterate over another to create dependent resources:

```
- id: databases
  forEach:
    - dbSpec: ${schema.spec.databases}
  template:
    apiVersion: db.example.com/v1
    kind: Database
    metadata:
      name: ${schema.metadata.name + '-' + dbSpec.name}
    spec:
      storage: ${dbSpec.storage}

- id: backupJobs
  forEach:
    - db: ${databases}  # iterate over the collection
  template:
    kind: CronJob
    metadata:
      name: ${schema.metadata.name + '-backup-' + db.metadata.name}
    spec:
      schedule: "0 2 * * *"
      jobTemplate:
        spec:
          template:
            spec:
              containers:
                - name: backup
                  env:
                    - name: DB_HOST
                      value: ${db.status.endpoint}
```

### Dependency Behavior[​](https://kro.run/docs/concepts/rgd/resource-definitions/collections/#dependency-behavior "Direct link to Dependency Behavior")

Like references in templates, resource references in `forEach` expressions create edges in the dependency graph. Any reference between a resource and a collection - whether in the `forEach` expression or in the template - causes kro to wait for the referenced collection to be fully reconciled and ready (`readyWhen` resolves to true) before proceeding:

```
resources:
  # Collection A: creates multiple databases
  - id: databases
    forEach:
      - dbSpec: ${schema.spec.databases}
    template:
      kind: Database
      metadata:
        name: ${schema.metadata.name + '-' + dbSpec.name}
      # ...

  # Collection B: depends on Collection A
  # kro waits for ALL databases to be ready before creating backup jobs
  - id: backupJobs
    forEach:
      - db: ${databases}
    template:
      kind: CronJob
      metadata:
        name: ${schema.metadata.name + '-backup-' + db.metadata.name}
      # ...

  # Resource C: depends on Collection B
  # kro waits for ALL backup jobs to be ready before creating the summary
  - id: summary
    template:
      kind: ConfigMap
      metadata:
        name: ${schema.metadata.name + '-summary'}
      data:
        backupCount: ${string(size(backupJobs))}
```

For collections, `readyWhen` uses the `each` keyword for per-item readiness checks. The collection is ready when **all** items pass **all** readyWhen expressions (AND semantics):

```
- id: workerPods
  forEach:
    - worker: ${schema.spec.workers}
  readyWhen:
    - ${each.status.phase == 'Running'}  # Per-item check using `each`
  template:
    kind: Pod
    metadata:
      name: ${schema.metadata.name + '-' + worker}
    spec:
      containers:
        - name: app
          image: ${schema.spec.image}
```

In this example, `workerPods` is only considered ready when **every** pod in the collection has `status.phase == 'Running'`. Dependent resources wait for this condition before proceeding.

important

If the collection is empty (zero items), it is considered ready.

For empty-collection readiness, see [Constraints & Gotchas](https://kro.run/docs/concepts/rgd/resource-definitions/collections/#constraints--gotchas) below.

tip

Without `readyWhen`, a collection is considered ready once all resources are created. Add `readyWhen` when dependent resources need specific conditions to be true, not just existence.

The `includeWhen` field operates on the collection as a whole. If the condition evaluates to `false`, the entire collection is skipped - no resources are created. You cannot use `includeWhen` to exclude individual items:

```
- id: backupJobs
  includeWhen:
    - ${schema.spec.backupsEnabled}  # all or nothing
  forEach:
    - dbSpec: ${schema.spec.databases}
  template:
    kind: CronJob
    metadata:
      name: ${schema.metadata.name + '-backup-' + dbSpec.name}
    # ...
```

To filter individual items from a collection, use `filter()` in the `forEach` expression:

```
- id: backupJobs
  forEach:
    # Only iterate over databases that have backups enabled
    - dbSpec: ${schema.spec.databases.filter(d, d.backupEnabled)}
  template:
    kind: CronJob
    metadata:
      name: ${schema.metadata.name + '-backup-' + dbSpec.name}
    # ...
```

For the all-or-nothing behavior of `includeWhen`, see [Constraints & Gotchas](https://kro.run/docs/concepts/rgd/resource-definitions/collections/#constraints--gotchas) below.

Collections are powerful but easy to misuse. Most issues come from identity collisions, dimension explosion from cartesian products, or assumptions about readiness and ordering. Use this section as a quick checklist when behavior looks surprising.

### External References[​](https://kro.run/docs/concepts/rgd/resource-definitions/collections/#external-references "Direct link to External References")

Collections are only supported for templated resources. A resource cannot use both `forEach` and `externalRef`.

### Dimension Explosion[​](https://kro.run/docs/concepts/rgd/resource-definitions/collections/#dimension-explosion "Direct link to Dimension Explosion")

Multiple iterators multiply the total number of resources. A small increase in each dimension can produce a large cartesian product, which slows reconciliation and increases cluster churn:

`3 regions × 5 tiers × 10 shards = 150 resources`

Keep iterator lists bounded and avoid combining large dimensions unless you intentionally want the expanded set.

### includeWhen Is Collection-wide[​](https://kro.run/docs/concepts/rgd/resource-definitions/collections/#includewhen-is-collection-wide "Direct link to includeWhen Is Collection-wide")

`includeWhen` applies to the entire collection. If it evaluates to `false`, the whole collection is skipped and **no items** are created. It cannot be used to filter individual items.

If you need per-item filtering, use `filter()` in the `forEach` expression instead.

### Empty Collections Are Ready[​](https://kro.run/docs/concepts/rgd/resource-definitions/collections/#empty-collections-are-ready "Direct link to Empty Collections Are Ready")

An empty array produces zero resources. The collection is still considered **ready**, because there are no items to wait on. This also applies to cartesian products where any iterator is empty (`2 × 0 = 0`).

If you expect downstream resources to wait on actual items, ensure the collection is non-empty or use additional conditions in `includeWhen`.

### Map Iteration Order Is Not Deterministic[​](https://kro.run/docs/concepts/rgd/resource-definitions/collections/#map-iteration-order-is-not-deterministic "Direct link to Map Iteration Order Is Not Deterministic")

Map iteration order in Go is randomized. If you iterate over map keys or values, convert the map to a sorted array first to make resource ordering stable. Unstable ordering can lead to noisy diffs and unexpected reconciliation churn.

For example:

```
# Given schema.spec.labels = {"app": "web", "env": "prod"}
forEach:
  - key: ${schema.spec.labels.map(k, k).sort()}
template:
  kind: ConfigMap
  metadata:
    name: ${schema.metadata.name + '-' + key}
  data:
    value: ${schema.spec.labels[key]}
```

Prefer Arrays Over Maps

When designing your API schema, prefer arrays over maps. Arrays offer better patching semantics (strategic merge patch), extensibility (items can grow new fields), deterministic ordering, and more predictable tooling behavior (kubectl, kustomize, helm).

### Identity Must Include All Iterator Dimensions[​](https://kro.run/docs/concepts/rgd/resource-definitions/collections/#identity-must-include-all-iterator-dimensions "Direct link to Identity Must Include All Iterator Dimensions")

Every iterator dimension must be represented in the resource identity. If any dimension is omitted, distinct items can collapse to the same resource and overwrite each other.

*   Namespaced Resources
*   Cluster-scoped Resources

```
# region + tier must appear in name or namespace
metadata:
  name: ${schema.metadata.name + '-' + region + '-' + tier}
  # OR: namespace: ${schema.metadata.name + '-' + region}
```

You can also split iterator dimensions across name and namespace:

```
metadata:
  name: ${schema.metadata.name + '-' + tier}
  namespace: ${schema.metadata.name + '-' + region}
```

### Collection Labels Are Managed[​](https://kro.run/docs/concepts/rgd/resource-definitions/collections/#collection-labels-are-managed "Direct link to Collection Labels Are Managed")

kro-managed collection labels are part of the public API. Do not modify them manually — changes will be reverted and can break collection bookkeeping.

Status expressions can reference collections to compute aggregate values. This lets you expose summary information about the collection in your custom resource's status:

```
spec:
  schema:
    apiVersion: v1alpha1
    kind: WorkerPool
    spec:
      workers: "[]string"
    status:
      total: ${size(workerPods)}
      running: ${size(workerPods.filter(w, w.status.phase == 'Running'))}
      ready: ${workerPods.all(w, w.status.phase == 'Running')}

  resources:
    - id: workerPods
      forEach:
        - worker: ${schema.spec.workers}
      template:
        kind: Pod
        metadata:
          name: ${schema.metadata.name + '-' + worker}
        # ...
```

Collections automatically stay in sync with their source data. When the source array changes, kro creates, updates, or deletes resources to match.

### Scaling Up[​](https://kro.run/docs/concepts/rgd/resource-definitions/collections/#scaling-up "Direct link to Scaling Up")

When items are added to the source array, kro creates new resources.

Example: if `workers` changes from `["alice", "bob"]` to `["alice", "bob", "charlie"]`, it creates one new pod for **charlie**.

### Scaling Down[​](https://kro.run/docs/concepts/rgd/resource-definitions/collections/#scaling-down "Direct link to Scaling Down")

When items are removed from the source array, kro automatically deletes the corresponding resources.

Example: if `workers` changes from `["alice", "bob", "charlie"]` to `["alice"]`, it deletes the pods for **bob** and **charlie**.

This cleanup happens automatically through kro's applyset mechanism - orphaned resources are pruned on each reconciliation.

### Empty Collections[​](https://kro.run/docs/concepts/rgd/resource-definitions/collections/#empty-collections "Direct link to Empty Collections")

An empty array results in zero resources - this is not an error: for example, `workers: []` means no pods are created (the collection exists but is empty).

This is useful for optional features that can be enabled by adding items to an initially empty array.

Empty collections are considered ready, since there are no items to wait on.

important

If any iterator in a cartesian product is empty, zero resources are created. For example, `regions: ["us-east"]` × `tiers: []` = zero deployments.

For more context on empty-collection behavior, see [Constraints & Gotchas](https://kro.run/docs/concepts/rgd/resource-definitions/collections/#constraints--gotchas) above.

### Drift Detection[​](https://kro.run/docs/concepts/rgd/resource-definitions/collections/#drift-detection "Direct link to Drift Detection")

kro continuously reconciles collection resources to match the desired state. If a resource is modified externally (drift), kro restores it:

`# 1. kro creates ConfigMap with data.prefix: "original"# 2. Someone manually changes data.prefix to "MODIFIED"# 3. kro detects the drift and restores data.prefix to "original"`

This applies to all fields that kro manages. Fields added by other controllers (not in kro's template) are preserved.

* * *

This section explains the internal mechanics for those who want to understand what happens under the hood. You don't need this to use collections effectively.

When you add `forEach` to a resource, kro treats that resource block as a template for multiple resources rather than a single one.

**At RGD validation time (static analysis)**, kro analyzes the `forEach` expressions to determine the element types. If `schema.spec.workers` is `[]string`, kro knows each iteration yields a string and types the iterator variable accordingly. This enables [static type checking](https://kro.run/docs/concepts/rgd/static-type-checking) of expressions inside the template before any resources are created.

**At instance reconciliation time (runtime)**, when the reconciler reaches a collection node in the dependency graph:

1.   Evaluates each forEach CEL expression to get the current collections
2.   Creates multiple resources - one per element (or combination)
3.   Compares against existing resources and creates, updates, or deletes as needed
4.   Evaluates `readyWhen` (if specified) across all collection resources before proceeding to dependent nodes

Each resource in the collection is tracked independently. If one resource fails to reconcile, the others continue - kro doesn't require all-or-nothing success within a collection.

note

In kro, each entry in `spec.resources` is a node in the dependency graph. A collection (a resource with `forEach`) is also a single node in the graph - it creates multiple resources at runtime, but dependencies treat it as one unit.

kro automatically applies labels to each resource in a collection to track membership and ordering. These labels are part of kro's public API and can be used for querying or debugging:

| Label | Description | Example |
| --- | --- | --- |
| `kro.run/node-id` | The resource ID from the RGD | `workerPods` |
| `kro.run/collection-index` | Position in the collection (0-indexed) | `0`, `1`, `2` |
| `kro.run/collection-size` | Total number of items in the collection | `3` |
| `kro.run/instance-id` | UID of the instance that owns this resource | `a1b2c3...` |

These labels enable:

*   **Querying**: Find all items in a collection with `kubectl get pods -l kro.run/node-id=workerPods`
*   **Ordering**: Understand item position via `collection-index`
*   **Debugging**: Trace resources back to their source instance and RGD

note

The combination of `instance-id` + `node-id` + `collection-index` uniquely identifies each collection item. These labels are managed by kro - do not modify them manually.

See [Constraints & Gotchas](https://kro.run/docs/concepts/rgd/resource-definitions/collections/#constraints--gotchas) for label management notes.

*   **[External References](https://kro.run/docs/concepts/rgd/resource-definitions/external-references)** - Reference existing resources not managed by kro
*   **[CEL Expressions](https://kro.run/docs/concepts/rgd/cel-expressions)** - Learn about CEL functions like `lists.range()`, `filter()`, and `map()`
*   **[Dependencies & Ordering](https://kro.run/docs/concepts/rgd/dependencies-ordering)** - Understand how collections affect the dependency graph
