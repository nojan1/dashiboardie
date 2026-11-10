# Requirements

A simple server dashboard. Main purpose is to show the status of different running "services".

Should be a web application. The frontend should use vue and the backend go. 

## Goals

* No authentication, access is handled by a policy in the proxy
* All services should be listed in the same view, regardless of source.
* Services listing should show: source, name, status, usage metrics (if possible)
* List should be filterable and searchable
* Sources are: podman (if it is a quadlet this take precedence), incus, nixos containers, selected systemd services
* It should be possible to start/stop/restart a service
* It should be possible to view logs for that service

