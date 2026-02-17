{{/*
Expand the name of the chart.
*/}}
{{- define "restapi-example.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "restapi-example.fullname" -}}
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
Create chart name and version as used by the chart label.
*/}}
{{- define "restapi-example.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "restapi-example.labels" -}}
helm.sh/chart: {{ include "restapi-example.chart" . }}
{{ include "restapi-example.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "restapi-example.selectorLabels" -}}
app.kubernetes.io/name: {{ include "restapi-example.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "restapi-example.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "restapi-example.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the name of the configmap
*/}}
{{- define "restapi-example.configmapName" -}}
{{- printf "%s-config" (include "restapi-example.fullname" .) }}
{{- end }}

{{/*
Create the name of the secret
*/}}
{{- define "restapi-example.secretName" -}}
{{- printf "%s-secret" (include "restapi-example.fullname" .) }}
{{- end }}

{{/*
Return the proper image name
*/}}
{{- define "restapi-example.image" -}}
{{- $tag := .Values.image.tag | default .Chart.AppVersion }}
{{- printf "%s:%s" .Values.image.repository $tag }}
{{- end }}

{{/*
Return the TLS secret name
*/}}
{{- define "restapi-example.tlsSecretName" -}}
{{- if .Values.config.tls.existingSecret }}
{{- .Values.config.tls.existingSecret }}
{{- else }}
{{- printf "%s-tls" (include "restapi-example.fullname" .) }}
{{- end }}
{{- end }}

{{/*
Return the Vault secret name
*/}}
{{- define "restapi-example.vaultSecretName" -}}
{{- if .Values.vault.existingSecret }}
{{- .Values.vault.existingSecret }}
{{- else }}
{{- printf "%s-vault" (include "restapi-example.fullname" .) }}
{{- end }}
{{- end }}

{{/*
Return the container port based on config
*/}}
{{- define "restapi-example.containerPort" -}}
{{- .Values.config.serverPort | default 8080 }}
{{- end }}

{{/*
Return the probe port based on config
*/}}
{{- define "restapi-example.probePort" -}}
{{- .Values.config.probePort | default 9090 }}
{{- end }}

{{/*
Return Prometheus annotations for metrics scraping
*/}}
{{- define "restapi-example.prometheusAnnotations" -}}
{{- if .Values.config.metricsEnabled }}
prometheus.io/scrape: "true"
prometheus.io/port: {{ include "restapi-example.probePort" . | quote }}
prometheus.io/path: "/metrics"
{{- end }}
{{- end }}
