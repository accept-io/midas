{{/*
Expand the name of the chart.
*/}}
{{- define "midas.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
Truncate to 63 chars because Kubernetes name fields have this limit.
*/}}
{{- define "midas.fullname" -}}
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
Create chart label value (name-version).
*/}}
{{- define "midas.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels applied to all resources.
*/}}
{{- define "midas.labels" -}}
helm.sh/chart: {{ include "midas.chart" . }}
{{ include "midas.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels — used by Service and Deployment selectors.
Keep stable; changing these requires a rolling replacement of pods.
*/}}
{{- define "midas.selectorLabels" -}}
app.kubernetes.io/name: {{ include "midas.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Resolve the name of the Secret that supplies sensitive env vars.
Returns the existingSecret name when set, otherwise the chart-managed Secret name.
*/}}
{{- define "midas.secretName" -}}
{{- if .Values.secret.existingSecret }}
{{- .Values.secret.existingSecret }}
{{- else }}
{{- include "midas.fullname" . }}
{{- end }}
{{- end }}

{{/*
Returns true when the chart should create a Secret (i.e. no existingSecret and
at least one inline secret value is non-empty).
*/}}
{{- define "midas.createSecret" -}}
{{- if and (not .Values.secret.existingSecret)
           (or .Values.secret.databaseUrl .Values.secret.authTokens .Values.secret.oidcClientSecret) }}
{{- true }}
{{- end }}
{{- end }}
