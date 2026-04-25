{{/*
Expand the name of the chart.
*/}}
{{- define "crd-schema-publisher.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "crd-schema-publisher.fullname" -}}
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
{{- define "crd-schema-publisher.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels.
*/}}
{{- define "crd-schema-publisher.labels" -}}
helm.sh/chart: {{ include "crd-schema-publisher.chart" . }}
{{ include "crd-schema-publisher.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels.
*/}}
{{- define "crd-schema-publisher.selectorLabels" -}}
app.kubernetes.io/name: {{ include "crd-schema-publisher.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use.
*/}}
{{- define "crd-schema-publisher.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "crd-schema-publisher.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Resolve the secret name.
Precedence: existingSecret.name > fullname (used by both externalSecret and secret.create).
*/}}
{{- define "crd-schema-publisher.secretName" -}}
{{- if .Values.existingSecret.name }}
{{- .Values.existingSecret.name }}
{{- else }}
{{- include "crd-schema-publisher.fullname" . }}
{{- end }}
{{- end }}

{{/*
Build the container image reference.
Digest takes precedence over tag; tag defaults to appVersion.
*/}}
{{- define "crd-schema-publisher.image" -}}
{{- if .Values.image.digest }}
{{- printf "%s@%s" .Values.image.repository .Values.image.digest }}
{{- else }}
{{- printf "%s:%s" .Values.image.repository (default .Chart.AppVersion .Values.image.tag) }}
{{- end }}
{{- end }}

{{/*
Common environment variables shared by both deployment and cronjob pods.
Optional Secret refs + config vars that apply to all modes.
*/}}
{{- define "crd-schema-publisher.commonEnvVars" -}}
{{- if or .Values.existingSecret.name .Values.externalSecret.enabled }}
- name: CLOUDFLARE_API_TOKEN
  valueFrom:
    secretKeyRef:
      name: {{ include "crd-schema-publisher.secretName" . }}
      key: CLOUDFLARE_API_TOKEN
- name: CLOUDFLARE_ACCOUNT_ID
  valueFrom:
    secretKeyRef:
      name: {{ include "crd-schema-publisher.secretName" . }}
      key: CLOUDFLARE_ACCOUNT_ID
- name: CF_PAGES_PROJECT
  value: {{ .Values.config.cfPagesProject | quote }}
{{- end }}
- name: OUTPUT_DIR
  value: {{ .Values.config.outputDir | quote }}
{{- if .Values.config.skipRender }}
- name: SKIP_RENDER
  value: {{ .Values.config.skipRender | quote }}
{{- end }}
{{- if .Values.config.basePath }}
- name: BASE_PATH
  value: {{ .Values.config.basePath | quote }}
{{- end }}
{{- end }}

{{/*
Validate that networkPolicy and ciliumNetworkPolicy are not both enabled.
*/}}
{{- define "crd-schema-publisher.validateNetworkPolicy" -}}
{{- if and .Values.networkPolicy.enabled .Values.ciliumNetworkPolicy.enabled }}
{{- fail "networkPolicy and ciliumNetworkPolicy are mutually exclusive — enable only one" }}
{{- end }}
{{- end }}

{{/*
Pod anti-affinity preset.
Generates preferred (soft) or required (hard) pod anti-affinity rules
using selector labels and kubernetes.io/hostname topology key.
Only applied when .Values.affinity is empty.
*/}}
{{- define "crd-schema-publisher.podAntiAffinity" -}}
{{- if and (not .Values.affinity) .Values.podAntiAffinityPreset }}
{{- if eq .Values.podAntiAffinityPreset "soft" }}
affinity:
  podAntiAffinity:
    preferredDuringSchedulingIgnoredDuringExecution:
      - weight: 100
        podAffinityTerm:
          labelSelector:
            matchLabels:
              {{- include "crd-schema-publisher.selectorLabels" . | nindent 14 }}
          topologyKey: kubernetes.io/hostname
{{- else if eq .Values.podAntiAffinityPreset "hard" }}
affinity:
  podAntiAffinity:
    requiredDuringSchedulingIgnoredDuringExecution:
      - labelSelector:
          matchLabels:
            {{- include "crd-schema-publisher.selectorLabels" . | nindent 12 }}
        topologyKey: kubernetes.io/hostname
{{- end }}
{{- end }}
{{- end }}
