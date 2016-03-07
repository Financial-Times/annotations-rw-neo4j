# Annotations Reader/Writer for Neo4j (annotations-rw-neo4j)

__An API for reading/writing annotations into Neo4j. Expects the annotations json supplied to be in the format that comes out of the annotations consumer.__

## Build & deployment etc:
*TODO*
_NB You will need to tag a commit in order to build, since the UI asks for a tag to build / deploy_
* [Jenkins view](http://ftjen10085-lvpr-uk-p:8181/view/annotations-private-rw)
* [Build and publish to forge](http://ftjen10085-lvpr-uk-p:8181/job/annotations-private-rw)
* [Deploy to test or production](http://ftjen10085-lvpr-uk-p:8181/job/annotations-private-rw)


## Installation & running locally
* `go get -u github.com/Financial-Times/annotations-private-rw`
* `cd $GOPATH/src/github.com/Financial-Times/annotations-private-rw`
* `go test ./...`
* `go install`
* `$GOPATH/bin/annotations-private-rw --neo-url={neo4jUrl} --port={port} --log-level={DEBUG|INFO|WARN|ERROR}`
_Except platformVersion, all arguments are optional.
--neo-url defaults to http://localhost:7474/db/data, which is the out of box url for a local neo4j instance.
--port defaults to 8080.
--log-level defaults to INFO
--platformVersion should have one of v1 or v2
See help text for other arguments._

## Endpoints

### PUT
/content/{annotatedContentId}/annotations

Each annotation is added with a relationship according to the predicate property from the payload.
If that is empty: a default MENTIONS relationship will be added between the content and a concept.

This operation acts as a replace - all existing annotations are removed, and the new ones are created - for the specified platformVersion. This is because we get these
annotations wholesale from the concept extraction service, which annotates the whole content on each publish.

Supplying an empty list as the request body will remove all annotations for the content.

A successful PUT results in 200.

We run queries in batches. If a batch fails, all failing requests will get a 500 server error response.

Invalid json body input will result in a 400 bad request response.

NB: annotations don't have identifiers themselves currently - the id in the json is the id of the concept that is annotating the content.

See [this doc](https://docs.google.com/document/d/1FE-JZDYJlKsxOIuQQkPwyyzcOkJQn8L3nNy1H8A8eDo) for more details.

Example:

    curl -XPUT -H "X-Request-Id: 123" -H "Content-Type: application/json" localhost:8080/content/3fa70485-3a57-3b9b-9449-774b001cd965/annotations --data
    "@annotations/examplePutBody.json"

NB: Although provenances are supplied is a list, we don't expect to get more than one provenance: we will take the scores from that one
and apply them to the relationship that we are creating for that annotation.

If there is no provenance, or the provenance is incomplete (e.g. no agent role) we'll still
create the relationship, it just won't have score, agent and time properties.

### GET
/content/{annotatedContentId}/annotations
This internal read should return what got written (i.e., this isn't the public annotations read API) - for the specified platformVersion.

If not found, you'll get a 404 response.

Empty fields are omitted from the response.
`curl -H "X-Request-Id: 123" localhost:8080/content/3fa70485-3a57-3b9b-9449-774b001cd965/annotations`

### DELETE
/content/{contentId}/annotations/

Deletes all the annotations with the specified platformVersion.

Will return 204 if successful, 404 if not found

`curl -XDELETE -H "X-Request-Id: 123" localhost:8080/3fa70485-3a57-3b9b-9449-774b001cd965/annotations`

NB: /content/{contentId}/annotations/mentions/{conceptId} also existed in the old annotations writer and was used to allow annotations to be removed in Spyglass (however it was not used because if the content is republished, we lose the fact an annotation was deleted). We have chosen not to replicate
that functionality in this app.


## Healthchecks
* Check connectivity [http://localhost:8080/__health](http://localhost:8080/__health)
* Ping: [http://localhost:8080/__ping](http://localhost:8080/__ping)
