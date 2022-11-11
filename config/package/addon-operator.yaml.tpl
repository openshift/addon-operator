apiVersion: apps/v1
kind: Deployment
metadata:
  name: addon-operator-manager
  namespace: openshift-addon-operator
  labels:
    app.kubernetes.io/name: addon-operator
  annotations:
    package-operator.run/phase: deploy
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: addon-operator
  strategy: {}
  template:
    metadata:
      labels:
        app.kubernetes.io/name: addon-operator
    spec:
      affinity:
        nodeAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
            - preference:
                matchExpressions:
                  - key: node-role.kubernetes.io/infra
                    operator: Exists
              weight: 100
      containers:
        - args:
            - --secure-listen-address=0.0.0.0:8443
            - --upstream=http://127.0.0.1:8080/
            - --tls-cert-file=/tmp/k8s-metrics-server/serving-certs/tls.crt
            - --tls-private-key-file=/tmp/k8s-metrics-server/serving-certs/tls.key
            - --logtostderr=true
            - --ignore-paths=/metrics,/healthz
          image: quay.io/openshift/origin-kube-rbac-proxy:4.10.0
          livenessProbe:
            initialDelaySeconds: 15
            periodSeconds: 20
            tcpSocket:
              port: 8443
          name: metrics-relay-server
          ports:
            - containerPort: 8443
          readinessProbe:
            initialDelaySeconds: 5
            periodSeconds: 10
            tcpSocket:
              port: 8443
          resources:
            limits:
              cpu: 100m
              memory: 30Mi
            requests:
              cpu: 100m
              memory: 30Mi
          volumeMounts:
            - mountPath: /tmp/k8s-metrics-server/serving-certs/
              name: tls
              readOnly: true
        - args:
            - --enable-leader-election
          image: quay.io/openshift/addon-operator:latest
          livenessProbe:
            httpGet:
              path: /healthz
              port: 8081
            initialDelaySeconds: 15
            periodSeconds: 20
          name: manager
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
      serviceAccountName: addon-operator
      tolerations:
        - effect: NoSchedule
          key: node-role.kubernetes.io/infra
      volumes:
        - name: tls
          secret:
            secretName: metrics-server-cert
