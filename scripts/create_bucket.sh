gsutil mb gs://doc-publisher-images
gsutil uniformbucketlevelaccess set on gs://doc-publisher-images
gsutil iam ch allUsers:objectViewer gs://doc-publisher-images
