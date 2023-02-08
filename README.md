# ListBucketResult Downloader

> Input ListBucketResult url to download resources from

## Explanation of Inputs

| Input                                     | Explanation                                                       | Required |
|-------------------------------------------|-------------------------------------------------------------------|----------|
| Bucket URL                                | The url for the public bucket containing ListBucketResult xml     | YES      |
| Site to grab bucket cookie authorizations | The url that supplies the CloudFront cookies to access the bucket | NO       |
| Bucket resource prefix                    | Prefix to narrow down the resources to download                   | NO       |
| Start download marker                     | The resource key as start index for downloading                   | NO       |
| Files to ignore regex                     | Files that contain this input to ignore in downloading            | NO       |

See [AWS documentation on ListObjectsV2](https://docs.aws.amazon.com/AmazonS3/latest/API/API_ListObjectsV2.html) for more information about querying buckets

## How to Run the Application

`go run .`  

## How to Build

`go build .`  