{{- define "postbrain.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "postbrain.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "postbrain.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "postbrain.labels" -}}
helm.sh/chart: {{ include "postbrain.chart" . }}
app.kubernetes.io/name: {{ include "postbrain.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "postbrain.selectorLabels" -}}
app.kubernetes.io/name: {{ include "postbrain.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "postbrain.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "postbrain.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{- define "postbrain.validateRouting" -}}
{{- if and (not .Values.ingress.enabled) (not .Values.httpRoute.enabled) -}}
{{- fail "One routing option must be enabled: set either ingress.enabled=true or httpRoute.enabled=true." -}}
{{- end -}}
{{- if and .Values.ingress.enabled .Values.httpRoute.enabled -}}
{{- fail "Enable only one routing option: ingress.enabled or httpRoute.enabled." -}}
{{- end -}}
{{- if and .Values.httpRoute.enabled (eq (len .Values.httpRoute.parentRefs) 0) -}}
{{- fail "httpRoute.parentRefs must be set when httpRoute.enabled=true." -}}
{{- end -}}
{{- end -}}

{{- define "postbrain.config" -}}
database:
  url: {{ .Values.config.database.url | quote }}
  auto_migrate: {{ .Values.config.database.auto_migrate }}
  max_open: {{ .Values.config.database.max_open }}
  max_idle: {{ .Values.config.database.max_idle }}
  connect_timeout: {{ .Values.config.database.connect_timeout | quote }}

embedding:
  backend: {{ .Values.config.embedding.backend | quote }}
  ollama_url: {{ .Values.config.embedding.ollama_url | quote }}
  text_model: {{ .Values.config.embedding.text_model | quote }}
  code_model: {{ .Values.config.embedding.code_model | quote }}
  summary_model: {{ .Values.config.embedding.summary_model | quote }}
  openai_api_key: {{ .Values.config.embedding.openai_api_key | quote }}
  request_timeout: {{ .Values.config.embedding.request_timeout | quote }}
  batch_size: {{ .Values.config.embedding.batch_size }}

server:
  addr: {{ .Values.config.server.addr | quote }}
  tls_cert: {{ .Values.config.server.tls_cert | quote }}
  tls_key: {{ .Values.config.server.tls_key | quote }}

migrations:
  expected_version: {{ .Values.config.migrations.expected_version }}

jobs:
  consolidation_enabled: {{ .Values.config.jobs.consolidation_enabled }}
  contradiction_enabled: {{ .Values.config.jobs.contradiction_enabled }}
  reembed_enabled: {{ .Values.config.jobs.reembed_enabled }}
  age_check_enabled: {{ .Values.config.jobs.age_check_enabled }}
  backfill_summaries_enabled: {{ .Values.config.jobs.backfill_summaries_enabled }}
  chunk_backfill_enabled: {{ .Values.config.jobs.chunk_backfill_enabled }}

oauth:
  base_url: {{ .Values.config.oauth.base_url | quote }}
  providers:
{{- range $name, $provider := .Values.config.oauth.providers }}
    {{ $name }}:
      enabled: {{ $provider.enabled }}
      client_id: {{ $provider.client_id | quote }}
      client_secret: {{ $provider.client_secret | quote }}
      scopes:
{{- range $scope := $provider.scopes }}
        - {{ $scope | quote }}
{{- end }}
{{- if hasKey $provider "instance_url" }}
      instance_url: {{ $provider.instance_url | quote }}
{{- end }}
{{- end }}
  server:
    auth_code_ttl: {{ .Values.config.oauth.server.auth_code_ttl | quote }}
    state_ttl: {{ .Values.config.oauth.server.state_ttl | quote }}
    token_ttl: {{ .Values.config.oauth.server.token_ttl | quote }}
    dynamic_registration: {{ .Values.config.oauth.server.dynamic_registration }}
  social:
    auto_create_users: {{ .Values.config.oauth.social.auto_create_users }}
    require_verified_email: {{ .Values.config.oauth.social.require_verified_email }}
{{- if .Values.config.oauth.social.allowed_email_domains }}
    allowed_email_domains:
{{- range $domain := .Values.config.oauth.social.allowed_email_domains }}
      - {{ $domain | quote }}
{{- end }}
{{- else }}
    allowed_email_domains: []
{{- end }}
{{- end -}}
