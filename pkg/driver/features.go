// Portions Copyright (C) 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0
package driver

type Feature string

const OCIExporter Feature = "OCI exporter"
const DockerExporter Feature = "Docker exporter"
const ContainerdExporter Feature = "Containerd exporter"

const CacheExport Feature = "cache export"
const MultiPlatform Feature = "multiple platforms"
