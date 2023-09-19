#!/bin/bash

# Delete merged-service. Even if it doesn't exist, it's fine
kubectl delete svc merged-service
kubectl delete deployment web-1-deploy
kubectl delete deployment web-2-deploy
kubectl delete deployment web-3-deploy

kubectl delete svc web-1-svc
kubectl delete svc web-2-svc
kubectl delete svc web-3-svc


kubectl delete svcmergerobj svcmergerobj-sample

kubectl apply -f deploy.yml 
kubectl apply -f service.yaml

# now make run
make manifests
make install
make run


