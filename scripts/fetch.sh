#!/bin/bash

#DOCUMENT="1_4OtBmq2gG8zFnqTlAvpHc1sshfkv4hw3z62vHs4crI"  # sample for tinkering
#LOCAL_NAME="sample_for_tinkering"

DOCUMENT="1yIZCzr5e5idePZwbe5g0udsy0nAwhpxucKffKqCy2Vw"
LOCAL_NAME="thoughts_on_iason_gabriel"

#DOCUMENT="1px3ivo6aFqAi0TA4u9oJkxwsry1D5GYv76GZ4nV00Rk"  # ground of optimization
#LOCAL_NAME="ground_of_optimization"

#DOCUMENT="1DJEooosbpX_Yeda61L412n8GmOymZ8PNFGtoMp4BLRE"  # search vs design
#LOCAL_NAME="search_vs_design"

go build || exit 1

./doc-publisher fetch googledoc "$DOCUMENT" \
    --output "out/${LOCAL_NAME}.googledoc"
