IMAGE := gcr.io/doc-publisher-341418/doc-publisher-backend
SERVICE := doc-publisher-backend	# the cloud run service
REGION := us-central1				# the region for deployment
ADDON := doc-publisher				# name of the Google Workspaces addon
PROJECT := doc-publisher-341418		# google cloud project

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

logs:
	xdg-open "https://console.cloud.google.com/run/detail/$(REGION)/$(SERVICE)/logs?project=$(PROJECT)"

logtail:
	gcloud logging read \
		"resource.type=cloud_run_revision AND resource.labels.service_name=$(SERVICE)" \
		--project $(PROJECT) \
		--limit 25

# Google workspace addons

create-addon:
	gcloud workspace-add-ons deployments create $(ADDON) \
		--deployment-file=addon.json

update-addon:
	gcloud workspace-add-ons deployments replace $(ADDON) \
		--deployment-file=addon.json

describe-addon:
	gcloud workspace-add-ons deployments describe $(ADDON)

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
