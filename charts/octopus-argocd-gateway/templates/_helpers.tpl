{{/*
Expand the name of the chart.
*/}}
{{- define "octopus-argocd-gateway.name" -}}
{{ .Values.nameOverride | default "octopus-argocd-gateway" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "octopus-argocd-gateway.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
The base image for the monitor, without any suffixes.
Defaults to the Chart Appversion.
*/}}
{{- define "octopus-argocd-gateway.image" -}}
{{- printf "%s/%s:%s" .Values.image.registry .Values.image.repository (.Values.image.tag | default (printf "v%s" .Chart.AppVersion)) }}
{{- end }}

{{/*
The complete image for the gateway, including any optional suffixes.
*/}}
{{- define "octopus-argocd-gateway.fullImage" -}}
{{- if .Values.image.tagSuffix }}
{{- printf "%s-%s" (include "octopus-argocd-gateway.image" .) .Values.image.tagSuffix }}
{{- else }}
{{- (include "octopus-argocd-gateway.image" .) }}
{{- end }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "octopus-argocd-gateway.labels" -}}
helm.sh/chart: {{ include "octopus-argocd-gateway.chart" . }}
{{ include "octopus-argocd-gateway.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | trunc 63 | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "octopus-argocd-gateway.selectorLabels" -}}
app.kubernetes.io/name: {{ include "octopus-argocd-gateway.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
The name of the config map to use for configuring the gateway
*/}}
{{- define "octopus-argocd-gateway.configMapName" -}}
{{- printf "%s-config" ( include "octopus-argocd-gateway.name" . ) }}
{{- end }}

{{/*
The name of the secret containing custom certificates that will be added to the gateway's cert pool
*/}}
{{- define "octopus-argocd-gateway.certificates.secretName" -}}
{{- printf "%s-certificates" ( include "octopus-argocd-gateway.name" . ) }}
{{- end }}

{{/*
The name of the secret containing custom certificates that will be added to the gateway's cert pool for registration
*/}}
{{- define "octopus-argocd-gateway.registration.certificates.secretName" -}}
{{- printf "%s-registration-certificates" ( include "octopus-argocd-gateway.name" . ) }}
{{- end }}

# Volume Mount Helpers

{{/*
The name of the config volume mount
*/}}
{{- define "octopus-argocd-gateway.config.volumeMountName" -}}
{{- printf "configuration-volume" }}
{{- end }}

{{/*
The path of the config volume mount
*/}}
{{- define "octopus-argocd-gateway.config.volumeMountPath" -}}
{{- printf "/octopus-argo-gateway" }}
{{- end }}

# Registration Helpers
{{- define "octopus-argocd-gateway.registration.hookAnnotationsWithSuccessDelete" -}}
"helm.sh/hook": "pre-upgrade,pre-install"
"helm.sh/hook-delete-policy": "before-hook-creation,hook-succeeded"
{{- end }}

{{/*
The name of the service account to use for the gateway
*/}}
{{- define "octopus-argocd-gateway.gateway.serviceAccountName" -}}
{{- if not .Values.gateway.serviceAccount.create }}
{{- required "If you aren't creating a service account, a valid .Values.gateway.serviceAccount.name is required!" .Values.registration.serviceAccount.name }}
{{- end }}
{{- .Values.gateway.serviceAccount.name | default (printf "%s" ( include "octopus-argocd-gateway.name" . )) }}
{{- end }}

{{/*
Create the name of the gateway role to use
*/}}
{{- define "octopus-argocd-gateway.gateway.roleName" -}}
{{- printf "%s-role" (include "octopus-argocd-gateway.gateway.serviceAccountName" .) }}
{{- end }}

{{/*
Create the name of the gateway role binding to use
*/}}
{{- define "octopus-argocd-gateway.gateway.roleBindingName" -}}
{{- printf "%s-binding" (include "octopus-argocd-gateway.gateway.serviceAccountName" .) }}
{{- end }}

{{/*
The name of the registration hook job
*/}}
{{- define "octopus-argocd-gateway.registration.jobName" -}}
{{- printf "%s-registration" ( include "octopus-argocd-gateway.name" . ) }}
{{- end }}

{{/*
The name of the service account to use for registration
*/}}
{{- define "octopus-argocd-gateway.registration.serviceAccountName" -}}
{{- if not .Values.registration.serviceAccount.create }}
  {{- required "If you aren't creating a service account, a valid .Values.registration.serviceAccount.name is required!" .Values.registration.serviceAccount.name }}
{{- end }}
{{- .Values.registration.serviceAccount.name | default (printf "%s-registration" ( include "octopus-argocd-gateway.name" . )) }}
{{- end }}

{{/*
Create the name of the registration role to use
*/}}
{{- define "octopus-argocd-gateway.registration.roleName" -}}
{{- printf "%s-role" (include "octopus-argocd-gateway.registration.serviceAccountName" .) }}
{{- end }}

{{/*
Create the name of the registration role binding to use
*/}}
{{- define "octopus-argocd-gateway.registration.roleBindingName" -}}
{{- printf "%s-binding" (include "octopus-argocd-gateway.registration.serviceAccountName" .) }}
{{- end }}

{{/*
The name of the secret containing the Octopus Deploy access token.
If serverAccessToken is not supplied, serverAccessTokenSecretName is required.
*/}}
{{- define "octopus-argocd-gateway.registration.secretName" -}}
{{- if .Values.registration.octopus.serverAccessToken -}}
  {{- .Values.registration.octopus.serverAccessTokenSecretName | default (printf "%s-registration-secret" ( include "octopus-argocd-gateway.name" . )) -}}
{{- else -}}
  {{- required "You must supply either registration.octopus.serverAccessToken, or registration.octopus.serverAccessTokenSecretName" .Values.registration.octopus.serverAccessTokenSecretName -}}
{{- end -}}
{{- end }}

{{/*
The key within the secret containing the Octopus Deploy access token.
Defaults to "token"
*/}}
{{- define "octopus-argocd-gateway.registration.secretKey" -}}
{{- .Values.registration.octopus.serverAccessTokenSecretKey | default "token" -}}
{{- end }}


{{/*Post Registration Helpers*/}}
{{/*
The name of the secret to store sensitive gateway post-registration configuration values
*/}}
{{- define "octopus-argocd-gateway.octopus.authentication.secretName" -}}
{{- printf "%s-octopus-auth-secret" ( include "octopus-argocd-gateway.name" . ) }}
{{- end }}

{{/*
The name of the automatic update cronjob
*/}}
{{- define "octopus-argocd-gateway.autoUpdate.jobName" -}}
{{- printf "%s-updater" ( include "octopus-argocd-gateway.name" . ) }}
{{- end }}

{{/*
The name of the service account to use for automatic updating
*/}}
{{- define "octopus-argocd-gateway.autoUpdate.serviceAccountName" -}}
{{- if not .Values.autoUpdate.serviceAccount.create }}
  {{- required "If you aren't creating a service account, a valid .Values.autoUpdate.serviceAccount.name is required!" .Values.registration.serviceAccount.name }}
{{- end }}
{{- .Values.autoUpdate.serviceAccount.name | default (printf "%s-updater" ( include "octopus-argocd-gateway.name" . )) }}
{{- end }}

{{/*
Create the name of the automatic updating role to use
*/}}
{{- define "octopus-argocd-gateway.autoUpdate.roleName" -}}
{{- printf "%s-role" (include "octopus-argocd-gateway.autoUpdate.serviceAccountName" .) }}
{{- end }}

{{/*
Create the name of the automatic updating role binding to use
*/}}
{{- define "octopus-argocd-gateway.autoUpdate.roleBindingName" -}}
{{- printf "%s-binding" (include "octopus-argocd-gateway.autoUpdate.serviceAccountName" .) }}
{{- end }}

{{/*
The name of the secret containing the Argo CD token.
If authenticationToken is not supplied, authenticationTokenSecretName is required.
*/}}
{{- define "octopus-argocd-gateway.argocd.authToken.secretName" -}}
{{- if .Values.gateway.argocd.authenticationToken -}}
  {{- .Values.gateway.argocd.authenticationTokenSecretName | default (printf "%s-argocd-auth-secret" ( include "octopus-argocd-gateway.name" . )) -}}
{{- else -}}
  {{- required "You must supply either gateway.argocd.authenticationToken, or gateway.argocd.authenticationTokenSecretName" .Values.gateway.argocd.authenticationTokenSecretName -}}
{{- end -}}
{{- end }}

{{/*
The key within the secret containing the Argo CD token.
Defaults to "token"
*/}}
{{- define "octopus-argocd-gateway.argocd.authToken.secretKey" -}}
{{- .Values.gateway.argocd.authenticationTokenSecretKey | default "token" -}}
{{- end }}

{{/*
The name of the secret containing per-project Argo CD auth tokens.
If projectAuthenticationSecretName is set it wins; otherwise a chart-managed
secret name is used.
*/}}
{{- define "octopus-argocd-gateway.argocd.projectAuthTokens.secretName" -}}
{{- .Values.gateway.argocd.projectAuthenticationSecretName | default (printf "%s-argocd-project-auth-tokens" ( include "octopus-argocd-gateway.name" . )) -}}
{{- end }}

{{/*
Renders "true" when per-project Argo CD auth tokens should be projected into
the gateway container. Truthy whenever either projectAuthentication has
entries or projectAuthenticationSecretName names an external secret.
*/}}
{{- define "octopus-argocd-gateway.argocd.projectAuthTokens.enabled" -}}
{{- if or .Values.gateway.argocd.projectAuthentication .Values.gateway.argocd.projectAuthenticationSecretName -}}true{{- end -}}
{{- end }}

{{/*
The name of the metrics Service (and ServiceMonitor)
*/}}
{{- define "octopus-argocd-gateway.metrics.serviceName" -}}
{{- printf "%s-metrics" ( include "octopus-argocd-gateway.name" . ) }}
{{- end }}

{{/*
Validate metrics configuration
*/}}
{{- define "octopus-argocd-gateway.metrics.validate" -}}
{{- if and .Values.metrics.enabled (eq (int .Values.metrics.port) 0) -}}
{{- fail "metrics.port must not be 0 when metrics.enabled is true" -}}
{{- end -}}
{{- end }}
