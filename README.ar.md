# صور QuenchWorks

[English](README.md) · **العربية** · [Español](README.es.md)

<p align="center">
  <a href="https://quench-works.com/images"><img src="https://img.shields.io/endpoint?url=https://quench-works.com/api/v1/badge/images.json" alt="images"></a>
  <a href="https://quench-works.com/charts"><img src="https://img.shields.io/endpoint?url=https://quench-works.com/api/v1/badge/charts.json" alt="charts"></a>
  <a href="https://quench-works.com/security"><img src="https://img.shields.io/endpoint?url=https://quench-works.com/api/v1/badge/cves.json" alt="open CVEs"></a>
  <a href="https://github.com/wolfi-dev"><img src="https://img.shields.io/endpoint?url=https://quench-works.com/api/v1/badge/wolfi.json" alt="built from source"></a>
  <a href="https://docs.sigstore.dev/"><img src="https://img.shields.io/endpoint?url=https://quench-works.com/api/v1/badge/cosign.json" alt="signed with cosign"></a>
  <a href="https://quench-works.com/images"><img src="https://img.shields.io/endpoint?url=https://quench-works.com/api/v1/badge/multiarch.json" alt="multi-arch"></a>
  <a href="https://artifacthub.io/packages/search?org=quenchworks"><img src="https://img.shields.io/endpoint?url=https://quench-works.com/api/v1/badge/artifacthub.json" alt="ArtifactHub"></a>
  <a href="https://github.com/quenchworks"><img src="https://img.shields.io/endpoint?url=https://quench-works.com/api/v1/badge/license.json" alt="license"></a>
</p>

مصنع الصور. يبني صور حاويات مُحصَّنة من المصدر على [Wolfi](https://github.com/wolfi-dev) باستخدام [melange](https://github.com/chainguard-dev/melange) و[apko](https://github.com/chainguard-dev/apko)، ويُخضعها لبوابة صارمة بصفر ثغرات (0-CVE)، ويوقّعها باستخدام cosign، وينشرها إلى GHCR.

<p align="center">
  <a href="https://quench-works.com"><img src="https://raw.githubusercontent.com/quenchworks/.github/main/profile/assets/demo.gif" alt="QuenchWorks في الطرفية: تشغيل صورة بصفر ثغرات، والتحقق منها باستخدام cosign، ونشر مخطط Helm، ومراقبة وصول الـ pod إلى حالة Running." width="760"></a>
</p>

**90+ صورة مُحصَّنة** للبنية التحتية التي تُشغّلها فعليًا. لا توجد ملفات Dockerfile. لا شيء موروث من توزيعة أخرى. مجانية وموقّعة ويُعاد بناؤها يوميًا.

جزء من [QuenchWorks](https://github.com/quenchworks)، البديل بصفر ثغرات لكتالوج Bitnami. تصفّح كل صورة، مع الإصدارات والبصمات، على [quench-works.com/images](https://quench-works.com/images).

## ما يُشحَن هنا

كل صورة في الكتالوج:

- **مبنية من المصدر**، بلا ملف Dockerfile، ولا شيء منقول من توزيعة أخرى. وحيث يكون تجميع مصدر المنبع غير ممكن عمليًا في التكامل المستمر (ClickHouse، ScyllaDB، CockroachDB، Dragonfly، MongoDB)، نشحن الملف الثنائي الرسمي الخاص بالمشروع نفسه ونُحصّن القاعدة من حوله،
- تجتاز بوابة صارمة بـ **0 ثغرة قابلة للإصلاح** (Trivy، فشل عند القابل للإصلاح) قبل نشر أي شيء،
- تعمل كـ **مستخدم غير جذر (uid 1001)** على **نظام ملفات جذري للقراءة فقط**،
- تُشحَن كفهرس **متعدد المعماريات** (linux/amd64 + linux/arm64)، موقّع ومثبّت بالبصمة،
- تحمل **SBOM** وتوقيع **cosign** بلا مفتاح.

يمتد الكتالوج عبر قواعد البيانات والذواكر المؤقتة والبحث والمتجهات والبث والتنسيق والمراقبة والبوابات والوكلاء وتخزين الكائنات والأسرار والهوية، بالإضافة إلى سجل حاويات (Harbor)، وGit ‏(Gitea)، والتكامل المستمر/البنية التحتية ككود (Atlantis).

## السحب والتحقق

```bash
# images are tagged by version (there is no :latest); swap redis:8.8.0 for any image and version
docker pull ghcr.io/quenchworks/images/redis:8.8.0

cosign verify ghcr.io/quenchworks/images/redis:8.8.0 \
  --certificate-identity-regexp 'https://github.com/quenchworks/.+' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

## كيف يجري البناء

1. يقوم **melange** بتجميع التطبيق من المصدر إلى حزمة APK موقّعة.
2. يقوم **apko** بتجميع صورة بسيطة وغير جذرية ومتعددة المعماريات ويكتبها إلى ملف tar محلي.
3. يقوم **Trivy** بفحص ذلك الملف tar باستخدام `--exit-code 1 --ignore-unfixed`. ثغرة واحدة قابلة للإصلاح تُفشل البناء، ولا يُنشر أي شيء.
4. بمجرد اجتياز البوابة فقط، ينشر apko إلى `ghcr.io/quenchworks/images/<app>` كفهرس متعدد المعماريات.
5. يوقّع **cosign** البصمة (بلا مفتاح).
6. يُخبر إرسالٌ مستودع [charts](https://github.com/quenchworks/charts) بإعادة تثبيت المخطط المطابق على البصمة الجديدة.

يجري البناء عند كل تغيير **ومرة واحدة يوميًا**. إعادة البناء اليومية تلك هي الهدف: فحص نظيف يبقى صحيحًا غدًا بدلًا من أن يتقادم بهدوء.

## التخطيط

```
catalog.yaml                 source of truth: app, version, source, license, tier, status
apps/<app>/melange.yaml      build the package from source
apps/<app>/apko.yaml         assemble the minimal nonroot image
apps/<app>/test.sh           smoke test the built image
.github/workflows/           per-app build, scan, sign, dispatch
```

## إضافة تطبيق

أضف صفًا إلى `catalog.yaml`، ثم أنشئ `apps/<app>/` مع بناء melange، وتهيئة apko، واختبار. تُؤلَّف المخططات بشكل منفصل في مستودع [charts](https://github.com/quenchworks/charts)، من وثائق المنبع الخاصة بكل تطبيق. راجع [CONTRIBUTING](https://github.com/quenchworks/.github/blob/main/CONTRIBUTING.md).

## ملاحظة حول الترخيص

معظم الكتالوج نظيف من ناحية OSI. أربعة مخازن بيانات متاحة المصدر ومحمولة مع ملاحظة ترخيص بارزة في `catalog.yaml` وعلى الموقع، لأنها **ليست** برمجيات مفتوحة المصدر معتمدة من OSI: MongoDB وElasticsearch ‏(SSPL-1.0)، وCockroachDB وDragonfly ‏(BUSL-1.1). يسمّي كلٌّ منها البديل النظيف الذي نوصي به بدلًا منه: Valkey، OpenSearch، FerretDB + DocumentDB.

## الترخيص

MIT لتهيئات البناء والأدوات الخاصة بهذا المستودع. تحمل كل صورة مبنية ترخيص برمجيات المنبع الخاص بها، المُسجَّل في `catalog.yaml` وفي تسميات الصورة.
