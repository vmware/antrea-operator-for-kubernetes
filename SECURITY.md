# Security Procedures
The Antrea community holds security in the highest regard.
The community adopted this security disclosure policy to ensure vulnerabilities are responsibly handled.

## Reporting a Vulnerability

If you believe you have identified a vulnerability with the Antrea operator, please work with the maintainers to fix it
and disclose the issue responsibly. All security issues, confirmed or suspected, should be reported privately.
Please avoid using github issues, and instead report the vulnerability to projectantrea-maintainers@googlegroups.com.

A vulnerability report should be filed if any of the following applies:
* You have discovered and confirmed a vulnerability in the Antrea operator.
* You believe the Antrea operator might be vulnerable to some published [CVE](https://cve.mitre.org/cve/).
* You have found a potential security flaw in the Antrea operator but you're not yet sure whether there's a viable attack vector.
* You have confirmed or suspect any of Antrea operator's dependencies has a vulnerability.

### Vulnerability report template
Provide a descriptive subject and include the following information in the body:
* Detailed steps to reproduce the vulnerability  (scripts, screenshots, manual procedures, etc.).
* Describe the effects of the vulnerability on the cluster and on the components managed by the Antrea operator
* How the vulnerability affects Antrea operator workflows.
* Potential attack vectors and an estimation of the attack surface, if applicable.
* Other software that was used to expose the vulnerability.
 
## Responding to a vulnerability

A coordinator is assigned to each reported security issue. The coordinator is a member from the Antrea operator maintainers team, and will drive the fix and disclosure process.
At the moment reports are received via email at projectantrea-maintainers@googlegroups.com.
The first steps performed by the coordinator are to confirm the validity of the report and send an embargo reminder to all parties involved.
Antrea operator maintainers and issue reporters will review the issue for confirmation of impact and determination of affected components.

With reference to the scale reported below, reported vulnerabilities will be disclosed and treated as regular issues if their issue risk is low (level 4 or higher in the scale).
For these lower-risk issues the fix process will proceed with the usual github workflow.

### Reference taxonomy for issue risk
1. Vulnerability must be fixed in master and any other supported branch.
2. Vulnerability must be fixed in master only for next release.
3. Vulnerability in experimental features or troubleshooting code.
4. Vulnerability without practical attack vector (e.g.: needs GUID guessing).
5. Not a vulnerability per se, but an opportunity to strengthen security (in code, architecture, protocols, and/or processes).
6. Not a vulnerability or a strengthening opportunity.
7. Vulnerability only exist in some PR or non-release branch.

## Developing a patch for a vulnerability

This part of the process applies only to confirmed vulnerabilities.
The reporter and Antrea operator maintainers, plus anyone they deem necessary to develop and validate a fix will be included the discussion.

**Please refrain from creating a PR for the fix!**

A fix is proposed as a patch to the current master branch, formatted with:
```
git format-patch --stdout HEAD~1 > path/to/local/file.patch
```
and then sent to projectantrea-maintainers@googlegroups.com.

**Please don't push the patch to an Antrea operator fork on your github account!**

Patch review will be performed via email. Reviewers will suggest modifications and/or improvements, and then pre-approve it for merging. 
Pre-approval will ensure patches can be fast-tracked through public code review later at disclosure time.
