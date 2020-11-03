---
name: Bug report
about: Tell us about a problem you are experiencing

---
**What steps did you take and what happened**
[A clear and concise description of what the bug is, and what commands you ran.  If you can, try to describe a "minimally reproducible scenario" so others can easily reproduce what you saw.]

**What did you expect to happen**

**Environment Details:**

- kubectl buildkit version (use `kubectl buildkit version`)
- Kubernetes version (use `kubectl version`)
- Where are you running kubernetes (e.g., bare metal, vSphere Tanzu, Cloud Provider xKS, etc.)
- Container Runtime and version (e.g. containerd `sudo ctr version` or dockerd `docker version` on one of your kubernetes worker nodes)

**Builder Logs**
[If applicable, an excerpt from `kubectl logs -l app=buildkit` from around the time you hit the failure may be very helpful]

**Dockerfile**
[If applicable, please include your Dockerfile or excerpts related to the failure]

**Vote on this request**

This is an invitation to the community to vote on issues.  Use the "smiley face" up to the right of this comment to vote.

- :+1: "I would like to see this bug fixed as soon as possible"
- :-1: "There are more important bugs to focus on right now"