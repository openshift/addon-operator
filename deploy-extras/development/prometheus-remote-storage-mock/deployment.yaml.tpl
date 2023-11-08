apiVersion: apps/v1
kind: Deployment
metadata:
  name: prometheus-remote-storage-mock
  namespace: prometheus-remote-storage-mock
  labels:
    app.kubernetes.io/name: prometheus-remote-storage-mock
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: prometheus-remote-storage-mock
  template:
    metadata:
      labels:
        app.kubernetes.io/name: prometheus-remote-storage-mock
    spec:
      containers:
      - name: mock
        image: quay.io/app-sre/prometheus-remote-storage-mock:${VERSION}
        ports:
        - containerPort: 1234
        resources:
          limits:
            cpu: 30m
            memory: 30Mi
          requests:
            cpu: 30m
            memory: 30Mi
