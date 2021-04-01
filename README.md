# Annotations Reader/Writer for Neo4j (annotations-rw-neo4j)
[![Circle CI](https://circleci.com/gh/Financial-Times/annotations-rw-neo4j.svg?style=shield)](https://circleci.com/gh/Financial-Times/annotations-rw-neo4j)[![Go Report Card](https://goreportcard.com/badge/github.com/Financial-Times/annotations-rw-neo4j)](https://goreportcard.com/report/github.com/Financial-Times/annotations-rw-neo4j) [![Coverage Status](https://coveralls.io/repos/github/Financial-Times/annotations-rw-neo4j/badge.svg)](https://coveralls.io/github/Financial-Times/annotations-rw-neo4j)

__A service and an API for reading/writing annotations into Neo4j. 
If the consumer is enabled: the messages are consumed from the queue, they get written into Neo4j, and finally, if the producer is also enabled, they got forwarded into the next (PostAnnotations) queue.
The above flow can be initiated by the PUT endpoint as well. In this case, the service expects the annotations json to be supplied in the format that comes out of the annotations consumer.__

## Build from source
* download the source code of the project in a directory of your choice
* `cd {the-chosen-directory}/annotations-rw-neo4j`
* `go build -mod=readonly`

## Running locally
* `{the-chosen-directory}/annotations-rw-neo4j [--help]`

You have the following options here:
- run neo4j and kafka locally (by docker, or as native apps)
- open a tunnel to one of your team clusters that your app can connect to
- disable the functionality that requires kafka by setting the env var `SHOULD_FORWARD_MESSAGES=false`

Command line options:
```
--neoUrl                  neo4j endpoint URL (env $NEO_URL) (default "http://localhost:7474/db/data")
--port                    Port to listen on (env $APP_PORT) (default 8080)
--batchSize               Maximum number of statements to execute per batch (env $BATCH_SIZE) (default 1024)
--logLevel                Logging level (DEBUG, INFO, WARN, ERROR) (env $LOG_LEVEL) (default "INFO")
--lifecycleConfigPath     Json Config file - containing two config maps: one for originHeader to lifecycle, another for lifecycle to platformVersion mappings.  (env $LIFECYCLE_CONFIG_PATH) (default "annotation-config.json")
--zookeeperAddress        Address of the zookeeper service (env $ZOOKEEPER_ADDRESS) (default "localhost:2181")
--shouldConsumeMessages   Boolean value specifying if this service should consume messages from the specified topic (env $SHOULD_CONSUME_MESSAGES)
--consumerGroup           Kafka consumer group name (env $CONSUMER_GROUP)
--consumerTopic           Kafka consumer topic name (env $CONSUMER_TOPIC)
--brokerAddress           Kafka address (env $BROKER_ADDRESS) (default "localhost:9092")
--producerTopic           Topic to which received messages will be forwarded (env $PRODUCER_TOPIC) (default "PostPublicationMetadataEvents")
--shouldForwardMessages   Decides if annotations messages should be forwarded to a post publication queue (env $SHOULD_FORWARD_MESSAGES) (default true)
--appName                 Name of the service (env $APP_NAME) (default "annotations-rw")
```

## Running tests locally
* Run unit tests only: `go test -race ./...`
* Run unit and integration tests:
    ```
    docker-compose -f docker-compose-tests.yml up -d --build && \
    docker logs -f test-runner && \
    docker-compose -f docker-compose-tests.yml down -v
    ```

## Endpoints

### PUT
/content/{annotatedContentId}/annotations/{annotations-lifecycle}

Each annotation is added with a relationship according to the predicate property from the payload.
The predicate property is required and it's value has to be equal to one of the keys from the `relations` map in annotations/model.go

This operation acts as a replace - all existing annotations are removed, and the new ones are created - for the specified annotations-lifecycle.
Supplying an empty list as the request body will remove all annotations for the content.

A successful PUT results in 201.

We run queries in batches. If a batch fails, all failing requests will get a 500 server error response.

Invalid json body input will result in a 400 bad request response.

NB: annotations don't have identifiers themselves currently - the id in the json is the id of the concept that is annotating the content.

See [this doc](https://docs.google.com/document/d/1FE-JZDYJlKsxOIuQQkPwyyzcOkJQn8L3nNy1H8A8eDo) for more details.

Example:

    curl -XPUT -H "X-Request-Id: 123" -H "Content-Type: application/json" localhost:8080/content/3fa70485-3a57-3b9b-9449-774b001cd965/annotations/annotations-v1 --data
    "@annotations/examplePutBody.json"

NB: Although provenances are supplied is a list, we don't expect to get more than one provenance: we will take the scores from that one
and apply them to the relationship that we are creating for that annotation.

If there is no provenance, or the provenance is incomplete (e.g. no agent role) we'll still
create the relationship, it just won't have score, agent and time properties.

### GET
/content/{annotatedContentId}/annotations/{annotations-lifecycle}
This internal read should return what got written (i.e., this isn't the public annotations read API) - for the specified annotations-lifecycle.

If not found, you'll get a 404 response.

Empty fields are omitted from the response.
`curl -H "X-Request-Id: 123" localhost:8080/content/3fa70485-3a57-3b9b-9449-774b001cd965/annotations/annotations-v1`

### DELETE
/content/{contentId}/annotations/{annotations-lifecycle}

Deletes all the annotations with the specified annotations-lifecycle.

Will return 204 if successful, 404 if not found

`curl -XDELETE -H "X-Request-Id: 123" localhost:8080/3fa70485-3a57-3b9b-9449-774b001cd965/annotations/annotations-v1`

NB: /content/{contentId}/annotations/mentions/{conceptId} also existed in the old annotations writer and was used to allow annotations to be removed in Spyglass (however it was not used because if the content is republished, we lose the fact an annotation was deleted). We have chosen not to replicate
that functionality in this app.


## Admin Endpoints
* Health checks: [http://localhost:8080/__health](http://localhost:8080/__health)
* Good to go: [http://localhost:8080/__gtg](http://localhost:8080/__gtg)
* Build info: [http://localhost:8080/__build-info](http://localhost:8080/__build-info)
* Ping: [http://localhost:8080/__ping](http://localhost:8080/__ping)
