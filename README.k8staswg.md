container images built by the k8stopologyawareschedwg
=====================================================

### Problem description

This workgroup is sending changes to the upstream scheduler-plugins repository
to enable or improve the topology-aware scheduling. All the changes we are proposing
are meant to be eventually merged in the scheduler-plugins repository, and to find
their way in the container images released by that project.

However, there could be a time window on which there are changes merged in the scheduler-plugins repo,
but there is not yet a release cut, so there is no a container image with the changes.

There are two main cases on which this may happen

- we have a fix on top of a stable branch, but scheduler-plugins didn't do a patch release yet. (e.g. 0.20.10, 0.20.11...)
- we have a change merged on the `main` branch during the development cycle, and we would like to enable early experimentation
  and feedback before a new branch is created and before a release is cut.

For obvious reasons, we want to clearly signal the users which image is from which source, either upstream-built image
from the scheduler plugins or a image built by this workgroup from the upstream source but with extra patches on top.

### container images build policy

- As default choice, this workgroup always recommends to use upstream container images from the scheduler-plugins source
- To enable early consumption of fixes and new features, this workgroup provides extra container images. These images are built only when needed (to enable consumption of these changes) and not regularly, by design.
- The container images built by this workgroup are available on [the quay.io registry](https://quay.io/organization/k8stopologyawareschedwg)
- [The quay.io registry](https://quay.io/organization/k8stopologyawareschedwg) only holds images built by this workgroup, we don't mirror images from the scheduler-plugin upstream repo.

### container images tags policy

- if a container image is built from a snapshot from main/master, we use the tag format `v0.0.DATESEQ` where `DATE` is `YYYYMMDD` and `SEQ` is a two-digit number.
  For example, the first build from November, 1st is v0.0.2021110101 while the third build of October, 15 is v0.0.2021101503
- if a container image adds extra fixes on top of stable branches, we append two digits to the micro version. So we will have `v0.20.1001` `v0.20.1002` ... `v0.20.1009` ...

