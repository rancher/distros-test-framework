// Run from the repository root: groovy scripts/tests/post_release_captain_test.groovy
def pipeline = new File(args ? args[0] : 'scripts/Jenkinsfile_post_release_captain')

def runCaptain = { Map overrides = [:], boolean failChild = false ->
  def inputs = [
    PRODUCT_NAME: 'rke2', CHANNELS: 'latest', VERSIONS: 'v1.36.0+rke2r1',
    INSTALL_MODE: 'auto', HOSTNAME_PREFIX: 'rpmcheck', REPORT_TO_QASE: 'false',
    QA_INFRA_REF: 'candidate', QA_INFRA_REPO: 'example/qa-infra-automation',
    REPO: 'https://github.com/example/distros-test-framework.git', BRANCH: 'candidate',
    AWS_SSH_KEY_NAME: 'managed-key', DRY_RUN: false, PROPAGATE_CHILD_FAILURES: true
  ] + overrides
  def state = [builds: [], batches: [], messages: [], envEntries: []]
  def environment = inputs.collectEntries { key, value -> [key, value.toString()] }
  def binding = new Binding([
    env: environment, params: inputs, currentBuild: [:],
    scm: [extensions: [], userRemoteConfigs: [[url: inputs.REPO]]],
    node: { Closure body -> body() },
    wrap: { Map options, Closure body -> body() },
    withFolderProperties: { Closure body -> body() },
    withEnv: { List values, Closure body ->
      state.envEntries.addAll(values)
      values.each { entry ->
        def pair = entry.split('=', 2)
        environment[pair[0]] = pair[1]
      }
      body()
    },
    stage: { String name, Closure body -> body() },
    dir: { String path, Closure body -> body() },
    deleteDir: {}, checkout: { Map options -> },
    string: { Map parameter -> parameter },
    echo: { message -> state.messages << message.toString() },
    error: { message -> throw new IllegalArgumentException(message.toString()) },
    parallel: { Map jobs ->
      state.batches << jobs.size()
      jobs.values().each { it() }
    },
    build: { Map call ->
      state.builds << [job: call.job.toString(), propagate: call.propagate,
                      parameters: call.parameters.collectEntries { [(it.name): it.value.toString()] }]
      if (failChild && call.propagate) {
        throw new IllegalStateException('Child failed')
      }
      [result: failChild ? 'FAILURE' : 'SUCCESS']
    }
  ])
  new GroovyShell(binding).evaluate(pipeline)
  state
}

def expectFailure = { String message, Closure body ->
  try {
    body()
    assert false: "Expected failure: ${message}"
  } catch (IllegalArgumentException | IllegalStateException failure) {
    assert failure.message.contains(message)
  }
}

def rke2 = runCaptain([
  CHANNELS: 'testing, latest stable latest', VERSIONS: 'v1.35.0+rke2r1 v1.36.0+rke2r1',
  REPORT_TO_QASE: 'true', QASE_PROJECT_ID: 'EXAMPLE', QASE_RUN_ID: '123',
  AWS_SSH_PEM_KEY: 'do-not-forward', AWS_SSH_PEM_KEY_ARM: 'do-not-forward'
])
assert rke2.builds.size() == 12
assert rke2.batches == [4, 4, 4]
def expectedIDs = [testing: ['268', '269'], latest: ['212', '217'], stable: ['214', '218']]
rke2.builds.each { call ->
  def p = call.parameters
  def arm = call.job.contains('_arm_')
  assert call.job == "rke2-tests/rke2_validate_cluster${arm ? '_arm' : ''}_rpm_qainfra"
  assert p.INSTALL_MODE == 'INSTALL_RKE2_VERSION'
  assert p.INSTALL_VERSION in ['v1.35.0+rke2r1', 'v1.36.0+rke2r1']
  assert p.QASE_TEST_CASE_ID == expectedIDs[p.INSTALL_CHANNEL][arm ? 1 : 0]
  assert p.QASE_RUN_ID == '123'
  assert p.PROVISIONER_MODULE == 'qainfra'
  assert p.QA_INFRA_REF == 'candidate'
  assert p.QA_INFRA_REPO == 'example/qa-infra-automation'
  assert p.BRANCH == 'candidate' && p.REPO.endsWith('/example/distros-test-framework.git')
  assert p.AWS_SSH_KEY_NAME == 'managed-key'
  assert !p.keySet().any { it.startsWith('AWS_SSH_PEM_KEY') }
  assert !p.keySet().intersect(['TFVARS', 'ARCH', 'NODE_OS', 'SSH_USER', 'TEST_ARGS', 'KUBE_CONFIG'])
}
assert rke2.builds.collect { "${it.job}/${it.parameters.INSTALL_CHANNEL}/${it.parameters.INSTALL_VERSION}" }.unique().size() == 12
assert !rke2.envEntries.any { it.contains('do-not-forward') }
assert rke2.envEntries.contains('AWS_SSH_PEM_KEY=')

def k3s = runCaptain([PRODUCT_NAME: 'k3s', VERSIONS: 'v1.36.0+k3s1', REPORT_TO_QASE: 'true'])
assert k3s.builds*.job == ['k3s-tests/k3s_validate_cluster_rpm_qainfra', 'k3s-tests/k3s_validate_cluster_arm_rpm_qainfra']
assert k3s.builds*.parameters*.INSTALL_MODE == ['INSTALL_K3S_VERSION', 'INSTALL_K3S_VERSION']
assert k3s.builds*.parameters*.QASE_TEST_CASE_ID == ['215', '219']

def defaults = runCaptain([BRANCH: '', QA_INFRA_REF: '', QA_INFRA_REPO: '', INSTALL_MODE: ''])
assert defaults.builds.every { it.parameters.BRANCH == 'qa-infra-RC-1' }
assert defaults.builds.every { it.parameters.QA_INFRA_REF == 'main' && it.parameters.QA_INFRA_REPO == '' }
assert defaults.builds.every { it.parameters.QASE_TEST_CASE_ID == '' }

def dry = runCaptain([DRY_RUN: true])
assert dry.builds.isEmpty()
assert dry.messages.count { it.startsWith('DRY_RUN=true') && it.contains('_rpm_qainfra') } == 2
expectFailure('CHANNELS or VERSIONS is empty') { runCaptain([CHANNELS: '']) }
expectFailure('Unsupported product') { runCaptain([PRODUCT_NAME: 'other']) }
expectFailure('requires PROVISIONER_MODULE=qainfra') { runCaptain([PROVISIONER_MODULE: 'legacy']) }
expectFailure('require INSTALL_K3S_VERSION') { runCaptain([PRODUCT_NAME: 'k3s', INSTALL_MODE: 'INSTALL_RKE2_VERSION']) }
expectFailure('require INSTALL_RKE2_VERSION') { runCaptain([INSTALL_MODE: 'INSTALL_RKE2_COMMIT']) }
expectFailure('No QASE mapping') { runCaptain([CHANNELS: 'unknown', REPORT_TO_QASE: 'true', DRY_RUN: true]) }
expectFailure('Child failed') { runCaptain([:], true) }
assert runCaptain([PROPAGATE_CHILD_FAILURES: false], true).builds.size() == 2
println 'PASS: RPM captain routing, parameters, Qase, dry-run, concurrency and failure handling'
