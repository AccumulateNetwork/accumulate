# Cyclops Deployment Scripts

**⚠️ REFERENCE COPIES ONLY ⚠️**

These are **reference copies** of the deployment scripts from the artifacts2 directory. 

## 🚨 **Important Usage Notes**

### **DO NOT RUN THESE SCRIPTS FROM HERE**
- These copies are for **documentation reference only**
- **Always run scripts from the original artifacts2 directory**
- Scripts depend on relative paths to artifacts and configuration files

### **Correct Usage Location**
```bash
# CORRECT - Run from artifacts directory
cd /home/paulsnow/accumulate-network/artifacts2
./deploy-cyclops-complete.sh

# INCORRECT - Do not run from docs directory
cd /home/paulsnow/go/src/gitlab.com/AccumulateNetwork/accumulate/docs/cyclops/scripts
./deploy-cyclops-complete.sh  # ❌ This will fail
```

## 📋 **Script Reference**

| Script | Size | Purpose |
|--------|------|---------|
| `deploy-cyclops-complete.sh` | 4.3KB | Master deployment orchestrator |
| `phase1-prep.sh` | 25KB | Phase 1: Preparation and validation |
| `phase2-deploy.sh` | 14KB | Phase 2: Directory deployment |
| `phase3-launch.sh` | 7KB | Phase 3: Node launch |
| `phase4-validate.sh` | 11KB | Phase 4: Validation |

## 📖 **Documentation**

For complete script documentation, see:
- [**Deployment Scripts Reference**](../cyclops-deployment-scripts-reference.md) - Complete script documentation
- [**Artifacts Deployment Guide**](../cyclops-artifacts-deployment-guide.md) - Usage instructions
- [**Deployment Phases**](../cyclops-deployment-phases.md) - Phase details

## 🔄 **Keeping Scripts Updated**

These reference copies should be updated when the original scripts change:

```bash
# Update reference copies (run from docs/cyclops/scripts directory)
cp /home/paulsnow/accumulate-network/artifacts2/*.sh .
```

---

**Remember: Always use the original scripts in artifacts2 for actual deployments!**
