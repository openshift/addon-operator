apiVersion: apps/v1
kind: Deployment
metadata:
  name: addon-operator-webhooks
  namespace: addon-operator
  labels:
    app.kubernetes.io/name: addon-operator-webhook-server
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: addon-operator-webhook-server
  strategy:
    rollingUpdate:
      maxSurge: 25%
      maxUnavailable: 1
    type: RollingUpdate
  template:
    metadata:
      labels:
        app.kubernetes.io/name: addon-operator-webhook-server
    spec:
      serviceAccountName: addon-operator
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
            - matchExpressions:
              - key: node-role.kubernetes.io/infra
                operator: Exists
        podAntiAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
          - labelSelector:
              matchExpressions:
              - key: app.kubernetes.io/name
                operator: In
                values:
                - addon-operator-webhook-server
            topologyKey: "kubernetes.io/hostname"
      tolerations:
        - effect: NoSchedule
          key: node-role.kubernetes.io/infra
        - effect: NoSchedule
          key: node-role.kubernetes.io/master
      securityContext:
        runAsNonRoot: true
        seccompProfile:
          type: RuntimeDefault
      containers:
      - name: webhook
        image: quay.io/openshift/addon-operator-webhook:latest
        ports:
        - containerPort: 8080
        volumeMounts:
        - name: tls
          mountPath: "/tmp/k8s-webhook-server/serving-certs/"
          readOnly: true
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8081
          initialDelaySeconds: 15
          periodSeconds: 20
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8081
          initialDelaySeconds: 5
          periodSeconds: 10
        resources:
          limits:
            cpu: 200m
            memory: 50Mi
          requests:
            cpu: 100m
            memory: 30Mi
        securityContext:
          allowPrivilegeEscalation: false
          capabilities:
            drop:
            - ALL
      volumes:
      - name: tls
        secret:
          secretName: webhook-server-cert
