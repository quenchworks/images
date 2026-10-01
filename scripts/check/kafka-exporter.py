#!/usr/bin/env python3
# kafka-exporter: GitHub releases (danielqsj/kafka_exporter), newest patch per minor line.
from _lib import github, report_lines
report_lines("kafka-exporter", github("danielqsj/kafka_exporter"), 2)
