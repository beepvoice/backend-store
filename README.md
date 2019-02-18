# backend-store

Single Badger store to serve bite, transcription and any others. Is kinda bite-centric, so required values revolve around a Bite. Transacts through NATS.

## Environment Variables

Supply environment variables by either exporting them or editing ```.env```.

| ENV | Description | Default |
| ---- | ----------- | ------- |
| DBPATH | Path to store badger files in. Please make sure it exists. | /tmp/badger |
| NATS | Host and port of nats | nats://localhost:4222 |

## Key format

Takes in three variables: ```type```, ```key``` and ```start```. Type is the type of data to be inserted, e.g. ```bite```, ```bite_user``` or ```transcription```. Key could be some secret passphrase declaring you the Raj of British India for all I know. Start is the Epoch timestamp of the start of the Bite.

## NATS

Refer to protobuf definitions in ```backend-protobuf```.

| Name | What you do | Accepted Protobuf | Protobuf redundant fields | Response Protobuf | Response empty fields |
| ---- | ----------- | ----------------- | ------------------------- | ----------------- | --------------------- |
| new_store | Publish to | Store | - | - | - |
| request_store | Request | DataRequest | - | Response | client |
| scan_store | Request | ScanRequest | - | Response | client |


### new_store

Pushes the results of its operation to ```backend-subscribe```.

| Code | Message | Description |
| ---- | ------- | ----------- |
| 200 | Inserted bite's key | Store operation was successful |
| 400 | 400 Bad Request | Key could not be marshalled properly |
| 500 | 500 Internal Server Error | Error storing the bite in badger |
