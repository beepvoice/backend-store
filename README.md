# backend-store

Single Badger store to serve bite, transcription and any others. Is kinda bite-centric, so required values revolve around a Bite. Transacts through NATS.

## Key format

Takes in three variables: ```type```, ```key``` and ```start```. Type is the type of data to be inserted, e.g. ```bite```, ```bite_user``` or ```transcription```. Key could be some secret passphrase declaring you the Raj of British India for all I know. Start is the Epoch timestamp of the start of the Bite.

## NATS

Refer to protobuf definitions in ```backend-protobuf```.

| Name | What you do | Accepted Protobuf | Protobuf redundant fields | Response Protobuf | Response empty fields |
| ---- | ----------- | ----------------- | ------------------------- | ----------------- | --------------------- |
| new_store | Publish to | Store | - | - | - |
| request_store | Request | DataRequest | - | Response | client |
| scan_store | Request | ScanRequest | - | Response | client |
