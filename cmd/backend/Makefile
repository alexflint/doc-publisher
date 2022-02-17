IMAGE := gcr.io/doc-publisher-341418/doc-publisher-backend
SERVICE := doc-publisher-backend		# the cloud run service
REGION := us-central1					# the region for deployment
ADDON := doc-publisher					# name of the Google Workspaces addon

default: .bin/addon-backend

# Compilation

.bin/addon-backend: *.go
	CGO_ENABLED=0 GOOS=linux go build -o .bin/addon-backend

# Docker

image: .bin/addon-backend
	docker build . -t $(IMAGE)

push:
	docker push $(IMAGE)

configure-docker-credentials:
	gcloud auth configure-docker

# Cloud run

deploy:
	gcloud run deploy $(SERVICE) \
		--image $(IMAGE) \
		--region $(REGION) \
		--platform managed

status:
	gcloud run services list --platform managed

# Google workspace addons

create-addon:
	gcloud workspace-add-ons deployments create $(ADDON) \
		--deployment-file=addon.json

update-addon:
	gcloud workspace-add-ons deployments replace $(ADDON) \
		--deployment-file=addon.json

describe-addon:
	gcloud workspace-add-ons deployments describe $(ADDON) \
		--deployment-file=addon.json

# Cloud build

build-deploy:
	gcloud builds submit

# Secret management

encrypt-secrets:
	echo "Please enter the password from bitwarden under 'doc-publisher backend'..."
	go run ../crypt/*.go --encrypt secrets/*.json

decrypt-secrets:
	echo "Please enter the password from bitwarden under 'doc-publisher backend'..."
	go run ../crypt/*.go --decrypt secrets/*.encrypted