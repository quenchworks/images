#!/usr/bin/env python3
# kfp-persistence-agent: GitHub releases of kubeflow/pipelines (tags X.Y.Z, no v). Every kfp-* app builds from
# the same tag, so they move together. SDK releases are tagged sdk-* and drop out (no digit).
from _lib import github, report
report("kfp-persistence-agent", github("kubeflow/pipelines"), n=1)
