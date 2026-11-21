{{- define "hop.name" -}}
{{- .Chart.Name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "hop.fullname" -}}
{{- if contains .Chart.Name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{- define "hop.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
{{ include "hop.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "hop.selectorLabels" -}}
app.kubernetes.io/name: {{ include "hop.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "hop.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "hop.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{- define "hop.dbCluster" -}}
{{- printf "%s-db" (include "hop.fullname" .) }}
{{- end }}

{{- define "hop.dbSecret" -}}
{{- if .Values.postgres.cnpg.enabled }}
{{- printf "%s-app" (include "hop.dbCluster" .) }}
{{- else }}
{{- required "postgres.existingSecret is required when postgres.cnpg.enabled is false" .Values.postgres.existingSecret }}
{{- end }}
{{- end }}
