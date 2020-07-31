Enhancement: Support signed URLs

We added a middleware that verifies signed urls as generated by the owncloud-sdk. This allows directly downloading large files with browsers instead of using `blob://` urls, which eats memory ...

https://github.com/owncloud/ocis-proxy/issues/73
https://github.com/owncloud/ocis-proxy/pull/75
https://github.com/owncloud/ocis-ocs/pull/18
https://github.com/owncloud/owncloud-sdk/pull/504