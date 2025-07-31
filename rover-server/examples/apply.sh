# Copyright 2025 Deutsche Telekom IT GmbH
#
# SPDX-License-Identifier: Apache-2.0

export SERVER_URL="http://localhost:8080"

export ENV="poc-rgu"
export GROUP="eni"
export TEAM="hyper"
export TEAM_ID="$GROUP--$TEAM"

PRINT_REQUEST_BODY="true"


apply_rover() {
    local filename=$1
    if [ -z "$filename" ]; then
        echo "Usage: apply_rover <filename>"
        return 1
    fi
    if [ ! -f "$filename" ]; then
        echo "File not found: $filename"
        return 1
    fi

    content=$(cat "$filename"| yq -o=json)

    if [ "$PRINT_REQUEST_BODY" = "true" ]; then
        echo "===== Request body for $filename ====="
        echo "$content"
        echo ""
    fi

    name=$(echo "$content" | jq -r '.name')
    if [ -z "$name" ]; then
        echo "Name not found in the file: $filename"
        return 1
    fi

    url="$SERVER_URL/rovers/$TEAM_ID--$name"
    echo "Applying rover configuration to $url"
    response=$(curl -X PUT $url \
        -H "Content-Type: application/json" \
        -H "Authorization: Bearer $(token --env $ENV --group $GROUP --team $TEAM)" \
        -d "$content" \
        -s)

    echo "===== Response from server ====="
    echo $response | jq

    if [ $? -ne 0 ]; then
        return 1
    fi
    url="$SERVER_URL/rovers/$TEAM_ID--$name/status"
    echo "Getting status from $url"

     response=$(curl -X GET $url \
        -H "Content-Type: application/json" \
        -H "Authorization: Bearer $(token --env $ENV --group $GROUP --team $TEAM)" \
        -s)

    echo "===== Status response from server ====="
    echo $response | jq

    if [ $? -ne 0 ]; then
        return 1
    fi


}


if [ -z "$1" ]; then
   for file in *.yaml; do
        if [ -f "$file" ]; then
            echo "Applying rover from file: $file"
            apply_rover "$file"
        else
            echo "No JSON files found in the current directory."
        fi
    done
else 
    apply_rover "$1"
fi
