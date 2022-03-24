IMAGE := gcr.io/doc-publisher-341418/doc-publisher-backend
SERVICE := doc-publisher-backend	# the cloud run service
REGION := us-central1				# the region for deployment
ADDON := doc-publisher				# name of the Google Workspaces addon
PROJECT := doc-publisher-341418		# google cloud project

default:
	make image
	make push
	make deploy

# Compilation

.bin/addon-backend: *.go cards/*.json
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
		--project $(PROJECT) \
		--platform managed

status:
	gcloud run services list \
		--project $(PROJECT) \
		--platform managed

logs:
	xdg-open "https://console.cloud.google.com/run/detail/$(REGION)/$(SERVICE)/logs?project=$(PROJECT)"

logs-for-addon:
	echo "FYI: I have not yet gotten any logs to show up this way"
	xdg-open "https://console.cloud.google.com/logs/query;query=protoPayload.serviceName%3D%22gsuiteaddons.googleapis.com%22%0A?project=$(PROJECT)"

logtail:
	gcloud logging read \
		"resource.type=cloud_run_revision AND resource.labels.service_name=$(SERVICE)" \
		--project $(PROJECT) \
		--limit 25

# Google workspace addons

create-addon:
	gcloud workspace-add-ons deployments create $(ADDON) \
		--deployment-file=addon.json \
		--project $(PROJECT)

update-addon:
	gcloud workspace-add-ons deployments replace $(ADDON) \
		--deployment-file=addon.json \
		--project $(PROJECT)

describe-addon:
	gcloud workspace-add-ons deployments describe $(ADDON) \
		--project $(PROJECT)

# Secret management

SECRETS := $(shell ls secrets/* | grep -v "encrypted$$")

encrypt-secrets:
	echo "Please enter the password from bitwarden under 'doc-publisher backend'..."
	go run ../crypt/*.go --encrypt $(SECRETS)

decrypt-secrets:
	echo "Please enter the password from bitwarden under 'doc-publisher backend'..."
	go run ../crypt/*.go --decrypt secrets/*.encrypted

#
# UNUSED
#



# Cloud build

submit:
	gcloud builds submit
