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
      securityContext:
        runAsNonRoot: true
        seccompProfile:
          type: RuntimeDefault
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
      - name: tls
        secret:
          secretName: metrics-server-cert
      - configMap:
          defaultMode: 420
          items:
            - key: ca-bundle.crt
              path: tls-ca-bundle.pem
          name: trusted-ca-bundle
          optional: true
        name: trusted-ca-bundle
      containers:
      - name: metrics-relay-server
        image: quay.io/openshift/origin-kube-rbac-proxy:4.10.0
        args:
        - "--secure-listen-address=0.0.0.0:8443"
        - "--upstream=http://127.0.0.1:8080/"
        - "--tls-cert-file=/tmp/k8s-metrics-server/serving-certs/tls.crt"
        - "--tls-private-key-file=/tmp/k8s-metrics-server/serving-certs/tls.key"
        - "--logtostderr=true"
        - "--ignore-paths=/metrics,/healthz"
        - "--v=10"  ### only for dev
        volumeMounts:
        - name: tls
          mountPath: "/tmp/k8s-metrics-server/serving-certs/"
          readOnly: true
        ports:
        - containerPort: 8443
        readinessProbe:
          tcpSocket:
            port: 8443
          initialDelaySeconds: 5
          periodSeconds: 10
        livenessProbe:
          tcpSocket:
            port: 8443
          initialDelaySeconds: 15
          periodSeconds: 20
        resources:
          limits:
            cpu: 100m
            memory: 30Mi
          requests:
            cpu: 100m
            memory: 30Mi
      - name: manager
        image: quay.io/openshift/addon-operator:latest
        args:
        - --enable-leader-election
        volumeMounts:
        - mountPath: /etc/pki/ca-trust/extracted/pem
          name: trusted-ca-bundle
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
            cpu: 100m
            memory: 600Mi
          requests:
            cpu: 100m
            memory: 300Mi
