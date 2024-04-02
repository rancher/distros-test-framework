### Flags usage

##### Available flags

- `-installVersionOrCommit`
Used to specify the version or commit to install in a upgrade action. This flag is mandatory for upgrade tests such as:
```
UpgradeSUC
UpgradeManual
UpgradeNodeReplacement
```
- `-channel`
Used to specify the channel to install in a upgrade action. This flag is not mandatory.
- `-destroy`
Used to specify if the cluster should be destroyed after the test. This flag is not mandatory.
- `-sonobuoyVersion`
Used to specify the version of sonobuoy to install. This flag is not mandatory as it has a default value is used at:`TestSonobuoyMixedOS`
- `-certManagerVersion  -rancherHelmVersio  -rancherImageVersion`
Used to specify the version of cert-manager, rancher helm and rancher image to install. This flag is not mandatory as it has a default value is used at:`TestDeployCertManager and TestDeployRancher `

##### For template version flags, see here:
- [Version Bump Template](
https://github.com/rancher/distros-test-framework/blob/86278e7b2632f5d39ac91902c6097fddec58505c/docs/version_bump_template.md)
