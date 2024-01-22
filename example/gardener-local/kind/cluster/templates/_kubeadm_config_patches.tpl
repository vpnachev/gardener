{{- define "kubeadmConfigPatches" -}}
- |
  kind: ClusterConfiguration
  apiServer:
{{- if .Values.gardener.apiserverRelay.deployed }}
    certSANs:
      - localhost
      - 127.0.0.1
      - gardener-apiserver.relay.svc.cluster.local
{{- end }}
    extraArgs:
{{- if not .Values.gardener.controlPlane.deployed }}
      authorization-mode: RBAC,Node
{{- else }}
      authorization-mode: RBAC,Node,Webhook
      authorization-webhook-config-file: /etc/gardener/controlplane/auth-webhook-kubeconfig-{{ .Values.networking.ipFamily }}.yaml
      authorization-webhook-cache-authorized-ttl: "0"
      authorization-webhook-cache-unauthorized-ttl: "0"
{{- if .Values.gardener.serviceAccountIssuer }}
      service-account-issuer: "https://{{ .Values.gardener.serviceAccountIssuer }}"
      service-account-jwks-uri: "https://{{ .Values.gardener.serviceAccountIssuer }}/openid/v1/jwks"
{{- end }}
    extraVolumes:
    - name: gardener
      hostPath: /etc/gardener/controlplane/auth-webhook-kubeconfig-{{ .Values.networking.ipFamily }}.yaml
      mountPath: /etc/gardener/controlplane/auth-webhook-kubeconfig-{{ .Values.networking.ipFamily }}.yaml
      readOnly: true
      pathType: File
{{- end }}
- |
  apiVersion: kubelet.config.k8s.io/v1beta1
  kind: KubeletConfiguration
  maxPods: 500
  serializeImagePulls: false
  registryPullQPS: 10
  registryBurst: 20
{{- end -}}
