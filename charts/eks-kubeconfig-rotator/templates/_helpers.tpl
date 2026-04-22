{{/*
Expand the name of the chart.
*/}}
{{- define "eks-kubeconfig-rotator.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "eks-kubeconfig-rotator.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Chart label value.
*/}}
{{- define "eks-kubeconfig-rotator.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels applied to all managed objects.
*/}}
{{- define "eks-kubeconfig-rotator.labels" -}}
helm.sh/chart: {{ include "eks-kubeconfig-rotator.chart" . }}
{{ include "eks-kubeconfig-rotator.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: kany8s
{{- end }}

{{/*
Selector labels retained for backwards compatibility with the kustomize
overlay under config/eks-plugin/ (app.kubernetes.io/name == controller name).
*/}}
{{- define "eks-kubeconfig-rotator.selectorLabels" -}}
app.kubernetes.io/name: eks-kubeconfig-rotator
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Service account name. Defaults to the controller name so RBAC ClusterRole
subjects remain stable when serviceAccount.create is true.
*/}}
{{- define "eks-kubeconfig-rotator.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default "eks-kubeconfig-rotator" .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Compose full image reference from registry / repository / tag / digest.
Precedence:
  1. .Values.global.imageRegistry (if non-empty) overrides .Values.image.registry
  2. .Values.image.digest (if non-empty) overrides tag
  3. .Values.image.tag falls back to .Chart.AppVersion when empty
*/}}
{{- define "eks-kubeconfig-rotator.image" -}}
{{- $registry := .Values.image.registry -}}
{{- if .Values.global -}}
  {{- if .Values.global.imageRegistry -}}
    {{- $registry = .Values.global.imageRegistry -}}
  {{- end -}}
{{- end -}}
{{- $repository := .Values.image.repository -}}
{{- if .Values.image.digest -}}
{{- printf "%s/%s@%s" $registry $repository .Values.image.digest -}}
{{- else -}}
{{- $tag := .Values.image.tag | default .Chart.AppVersion -}}
{{- printf "%s/%s:%s" $registry $repository $tag -}}
{{- end -}}
{{- end }}

{{/*
Merge global.imagePullSecrets with chart-local imagePullSecrets into a single
list. Emits the full `imagePullSecrets:` key so callers can `include` at the
right indent inside Deployment.spec.template.spec.
*/}}
{{- define "eks-kubeconfig-rotator.imagePullSecrets" -}}
{{- $merged := list -}}
{{- if .Values.global -}}
  {{- range .Values.global.imagePullSecrets -}}
    {{- $merged = append $merged . -}}
  {{- end -}}
{{- end -}}
{{- range .Values.imagePullSecrets -}}
  {{- $merged = append $merged . -}}
{{- end -}}
{{- if $merged -}}
imagePullSecrets:
{{ toYaml $merged }}
{{- end -}}
{{- end }}

{{/* The mount path for the shared credentials file */}}
{{- define "eks-kubeconfig-rotator.aws.credentials.secretMountPath" -}}
{{- "/var/run/secrets/aws" -}}
{{- end }}

{{/* The path the shared credentials file is mounted */}}
{{- define "eks-kubeconfig-rotator.aws.credentials.filePath" -}}
{{- printf "%s/%s" (include "eks-kubeconfig-rotator.aws.credentials.secretMountPath" .) .Values.aws.credentials.secretKey -}}
{{- end }}

{{/*
Render manager flags as a YAML list. Only flags that differ from controller
defaults are emitted to keep Deployment args minimal.
*/}}
{{- define "eks-kubeconfig-rotator.args" -}}
{{- if .Values.args.leaderElect }}
- --leader-elect
{{- end }}
- --metrics-bind-address={{ .Values.args.metricsBindAddress }}
- --metrics-secure={{ .Values.args.metricsSecure }}
- --health-probe-bind-address={{ .Values.args.healthProbeBindAddress }}
{{- if .Values.args.enableHTTP2 }}
- --enable-http2
{{- end }}
{{- if .Values.args.watchNamespace }}
- --watch-namespace={{ .Values.args.watchNamespace }}
{{- end }}
- --refresh-before={{ .Values.args.refreshBefore }}
- --max-refresh-interval={{ .Values.args.maxRefreshInterval }}
- --failure-backoff={{ .Values.args.failureBackoff }}
{{- end }}
