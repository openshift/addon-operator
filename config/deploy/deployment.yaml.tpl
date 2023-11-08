apiVersion: apps/v1
kind: Deployment
metadata:
  name: addon-operator-manager
  namespace: addon-operator
  labels:
    app.kubernetes.io/name: addon-operator
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: addon-operator
  template:
    metadata:
      labels:
        app.kubernetes.io/name: addon-operator
    spec:
      serviceAccountName: addon-operator
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
            - matchExpressions:
              - key: node-role.kubernetes.io/infra
                operator: Exists
      tolerations:
      - effect: NoSchedule
        key: node-role.kubernetes.io/infra
      volumes:
      - name: tls-manager-metrics
        secret:
          secretName: tls-manager-metrics
      - configMap:
          defaultMode: 420
          items:
            - key: ca-bundle.crt
              path: tls-ca-bundle.pem
          name: trusted-ca-bundle
          optional: true
        name: trusted-ca-bundle
      securityContext:
        runAsNonRoot: true
        seccompProfile:
          type: RuntimeDefault
      containers:
      - name: manager
        ports:
        - containerPort: 8443
        image: quay.io/openshift/addon-operator:latest
        args:
        - --enable-leader-election
        - --metrics-addr=:8443
        - --metrics-cert-dir=/etc/tls/manager/metrics
        volumeMounts:
        - mountPath: /etc/pki/ca-trust/extracted/pem
          name: trusted-ca-bundle
          readOnly: true
        volumeMounts:
        - mountPath: /etc/tls/manager/metrics
          name: tls-manager-metrics
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
            cpu: 100m
            memory: 600Mi
          requests:
            cpu: 100m
            memory: 300Mi
        securityContext:
          allowPrivilegeEscalation: false
          capabilities:
            drop:
            - ALL
