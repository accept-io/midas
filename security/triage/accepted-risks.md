## Trivy DS-0026: Missing Docker HEALTHCHECK

**Severity:** LOW  
**Finding:** Dockerfile has no HEALTHCHECK instruction  
**Rationale:** MIDAS has /healthz and /readyz HTTP endpoints. Docker HEALTHCHECK omitted because:
  - Distroless base image has no wget/curl
  - Using :debug tag adds unnecessary bloat
  - Kubernetes liveness/readiness probes are the correct mechanism for orchestrated deployments
**Mitigation:** Document k8s probe configuration in deployment docs  
**Accepted by:** Philip O'Shaughnessy, 2026-03-27  
**Review date:** 2026-03-27  

## Trivy DS-0001: Base image uses :latest tag

**Severity:** MEDIUM  
**Finding:** FROM gcr.io/distroless/base-debian12:latest uses moving tag  
**Rationale:** Distroless images are Google-maintained, security-patched regularly. Using :latest ensures automatic security updates. Digest pinning would require manual updates on every CVE.  
**Mitigation:** 
  - Dependabot monitors base image updates
  - CI rebuilds on base image changes
  - Production deployments use immutable image tags (midas:v1.0.0)
**Accepted by:** Philip O'Shaughnessy, 2026-03-27  
**Review date:** 2026-06-27