#!/usr/bin/env python3
# zabbix-server: release tarballs live in per-line directories on cdn.zabbix.com, which is
# where build.conf fetches them. Same discovery as the recipe: the newest patch of each line
# (6.0 LTS, 7.0 LTS, 7.4); report_lines also names any newer line that appears.
from _lib import scrape, report_lines

BASE = "https://cdn.zabbix.com/zabbix/sources/stable/"
lines = [l for l in scrape(BASE, r'href="(\d+\.\d+)/"') if int(l.split(".")[0]) >= 6]
cands = []
for l in lines:
    cands += scrape(f"{BASE}{l}/", r'zabbix-(\d+\.\d+\.\d+)\.tar\.gz')
report_lines("zabbix-server", cands, 2)
