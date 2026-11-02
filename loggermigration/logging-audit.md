# Logging Audit

Generated on: 2026-05-18 17:04:18 UTC

---

## Summary

This document inventories logging coupling across the repository.

Goals:
- Identify concrete logger dependencies
- Identify public API exposure
- Estimate migration difficulty
- Plan phased logging abstraction rollout

---

# Direct logrus imports


## Direct logrus imports

```txt
cmd/containerd-shim-lcow-v2/main.go:21:	"github.com/sirupsen/logrus"
cmd/containerd-shim-lcow-v2/manager.go:28:	"github.com/sirupsen/logrus"
cmd/containerd-shim-lcow-v2/service/plugin/plugin.go:21:	"github.com/sirupsen/logrus"
cmd/containerd-shim-lcow-v2/service/service_sandbox.go:14:	"github.com/sirupsen/logrus"
cmd/containerd-shim-runhcs-v1/delete.go:14:	"github.com/sirupsen/logrus"
cmd/containerd-shim-runhcs-v1/exec_hcs.go:17:	"github.com/sirupsen/logrus"
cmd/containerd-shim-runhcs-v1/exec_wcow_podsandbox.go:17:	"github.com/sirupsen/logrus"
cmd/containerd-shim-runhcs-v1/main.go:17:	"github.com/sirupsen/logrus"
cmd/containerd-shim-runhcs-v1/serve.go:21:	"github.com/sirupsen/logrus"
cmd/containerd-shim-runhcs-v1/service_internal.go:19:	"github.com/sirupsen/logrus"
cmd/containerd-shim-runhcs-v1/start.go:22:	"github.com/sirupsen/logrus"
cmd/containerd-shim-runhcs-v1/task_hcs.go:22:	"github.com/sirupsen/logrus"
cmd/containerd-shim-runhcs-v1/task_wcow_podsandbox.go:26:	"github.com/sirupsen/logrus"
cmd/gcs-sidecar/main.go:20:	"github.com/sirupsen/logrus"
cmd/gcs/main.go:22:	"github.com/sirupsen/logrus"
cmd/gcstools/commoncli/common.go:9:	"github.com/sirupsen/logrus"
cmd/gcstools/generichook.go:17:	"github.com/sirupsen/logrus"
cmd/hooks/wait-paths/main.go:11:	"github.com/sirupsen/logrus"
cmd/ncproxy/main.go:8:	"github.com/sirupsen/logrus"
cmd/ncproxy/run.go:20:	"github.com/sirupsen/logrus"
cmd/ncproxy/server.go:20:	"github.com/sirupsen/logrus"
cmd/runhcs/container.go:31:	"github.com/sirupsen/logrus"
cmd/runhcs/main.go:14:	"github.com/sirupsen/logrus"
cmd/runhcs/shim.go:20:	"github.com/sirupsen/logrus"
cmd/runhcs/vm.go:20:	"github.com/sirupsen/logrus"
hcn/hcnendpoint.go:11:	"github.com/sirupsen/logrus"
hcn/hcnerrors.go:9:	"github.com/sirupsen/logrus"
hcn/hcnglobals.go:12:	"github.com/sirupsen/logrus"
hcn/hcnloadbalancer.go:10:	"github.com/sirupsen/logrus"
hcn/hcnnamespace.go:16:	"github.com/sirupsen/logrus"
hcn/hcnnetwork.go:11:	"github.com/sirupsen/logrus"
hcn/hcnroute.go:11:	"github.com/sirupsen/logrus"
hcn/hcnsupport.go:9:	"github.com/sirupsen/logrus"
internal/bridgeutils/commonutils/utilities.go:11:	"github.com/sirupsen/logrus"
internal/builder/vm/lcow/boot.go:20:	"github.com/sirupsen/logrus"
internal/builder/vm/lcow/confidential.go:26:	"github.com/sirupsen/logrus"
internal/builder/vm/lcow/devices.go:22:	"github.com/sirupsen/logrus"
internal/builder/vm/lcow/kernel_args.go:17:	"github.com/sirupsen/logrus"
internal/builder/vm/lcow/specs.go:24:	"github.com/sirupsen/logrus"
internal/builder/vm/lcow/topology.go:19:	"github.com/sirupsen/logrus"
internal/cmd/cmd.go:19:	"github.com/sirupsen/logrus"
internal/cmd/io.go:12:	"github.com/sirupsen/logrus"
internal/cmd/io_binary.go:20:	"github.com/sirupsen/logrus"
internal/cmd/io_npipe.go:18:	"github.com/sirupsen/logrus"
internal/computecore/computecore.go:11:	"github.com/sirupsen/logrus"
internal/controller/vm/vm.go:27:	"github.com/sirupsen/logrus"
internal/controller/vm/vm_wcow.go:16:	"github.com/sirupsen/logrus"
internal/devices/pnp.go:19:	"github.com/sirupsen/logrus"
internal/gcs-sidecar/bridge.go:19:	"github.com/sirupsen/logrus"
internal/gcs-sidecar/host.go:18:	"github.com/sirupsen/logrus"
internal/gcs-sidecar/vsmb.go:11:	"github.com/sirupsen/logrus"
internal/gcs/bridge.go:18:	"github.com/sirupsen/logrus"
internal/gcs/bridge_test.go:17:	"github.com/sirupsen/logrus"
internal/gcs/guestconnection.go:25:	"github.com/sirupsen/logrus"
internal/gcs/guestconnection_test.go:20:	"github.com/sirupsen/logrus"
internal/gcs/process.go:19:	"github.com/sirupsen/logrus"
internal/guest/bridge/bridge.go:20:	"github.com/sirupsen/logrus"
internal/guest/bridge/bridge_unit_test.go:19:	"github.com/sirupsen/logrus"
internal/guest/kmsg/kmsg.go:13:	"github.com/sirupsen/logrus"
internal/guest/network/netns.go:19:	"github.com/sirupsen/logrus"
internal/guest/runtime/hcsv2/container.go:18:	"github.com/sirupsen/logrus"
internal/guest/runtime/hcsv2/nvidia_utils.go:16:	"github.com/sirupsen/logrus"
internal/guest/runtime/hcsv2/process.go:21:	"github.com/sirupsen/logrus"
internal/guest/runtime/hcsv2/uvm.go:28:	"github.com/sirupsen/logrus"
internal/guest/runtime/runc/container.go:17:	"github.com/sirupsen/logrus"
internal/guest/runtime/runc/process.go:10:	"github.com/sirupsen/logrus"
internal/guest/runtime/runc/utils.go:17:	"github.com/sirupsen/logrus"
internal/guest/spec/spec.go:19:	"github.com/sirupsen/logrus"
internal/guest/spec/spec_devices.go:17:	"github.com/sirupsen/logrus"
internal/guest/stdio/stdio.go:14:	"github.com/sirupsen/logrus"
internal/guest/storage/crypt/crypt.go:17:	"github.com/sirupsen/logrus"
internal/guest/storage/overlay/overlay.go:17:	"github.com/sirupsen/logrus"
internal/guest/storage/scsi/scsi.go:18:	"github.com/sirupsen/logrus"
internal/guest/transport/devnull.go:9:	"github.com/sirupsen/logrus"
internal/guest/transport/log.go:6:	"github.com/sirupsen/logrus"
internal/guest/transport/vsock.go:14:	"github.com/sirupsen/logrus"
internal/hcs/callback.go:13:	"github.com/sirupsen/logrus"
internal/hcs/system.go:24:	"github.com/sirupsen/logrus"
internal/hcsoci/create.go:16:	"github.com/sirupsen/logrus"
internal/hcsoci/hcsdoc_wcow.go:18:	"github.com/sirupsen/logrus"
internal/hcsoci/network.go:13:	"github.com/sirupsen/logrus"
internal/hcsoci/resources.go:7:	"github.com/sirupsen/logrus"
internal/hns/hnsaccelnet.go:8:	"github.com/sirupsen/logrus"
internal/hns/hnsendpoint.go:10:	"github.com/sirupsen/logrus"
internal/hns/hnsfuncs.go:11:	"github.com/sirupsen/logrus"
internal/hns/hnsnetwork.go:10:	"github.com/sirupsen/logrus"
internal/hns/hnspolicylist.go:8:	"github.com/sirupsen/logrus"
internal/hns/hnssupport.go:6:	"github.com/sirupsen/logrus"
internal/jobcontainers/mounts.go:17:	"github.com/sirupsen/logrus"
internal/jobobject/iocp.go:15:	"github.com/sirupsen/logrus"
internal/layers/lcow.go:16:	"github.com/sirupsen/logrus"
internal/layers/wcow_mount.go:14:	"github.com/sirupsen/logrus"
internal/lcow/common.go:16:	"github.com/sirupsen/logrus"
internal/lcow/disk.go:12:	"github.com/sirupsen/logrus"
internal/lcow/scratch.go:13:	"github.com/sirupsen/logrus"
internal/log/context.go:6:	"github.com/sirupsen/logrus"
internal/log/format.go:12:	"github.com/sirupsen/logrus"
internal/log/hook.go:9:	"github.com/sirupsen/logrus"
internal/log/nopformatter.go:4:	"github.com/sirupsen/logrus"
internal/oc/exporter.go:4:	"github.com/sirupsen/logrus"
internal/oci/annotations.go:15:	"github.com/sirupsen/logrus"
internal/oci/annotations_test.go:15:	"github.com/sirupsen/logrus"
internal/oci/uvm.go:16:	"github.com/sirupsen/logrus"
internal/schemaversion/schemaversion.go:12:	"github.com/sirupsen/logrus"
internal/tools/networkagent/main.go:18:	"github.com/sirupsen/logrus"
internal/tools/rootfs/main.go:10:	"github.com/sirupsen/logrus"
internal/tools/rootfs/merge.go:20:	"github.com/sirupsen/logrus"
internal/tools/uvmboot/lcow.go:14:	"github.com/sirupsen/logrus"
internal/tools/uvmboot/main.go:12:	"github.com/sirupsen/logrus"
internal/tools/uvmboot/mounts.go:10:	"github.com/sirupsen/logrus"
internal/uvm/cimfs.go:18:	"github.com/sirupsen/logrus"
internal/uvm/computeagent.go:15:	"github.com/sirupsen/logrus"
internal/uvm/create.go:13:	"github.com/sirupsen/logrus"
internal/uvm/create_lcow.go:19:	"github.com/sirupsen/logrus"
internal/uvm/create_wcow.go:17:	"github.com/sirupsen/logrus"
internal/uvm/network.go:18:	"github.com/sirupsen/logrus"
internal/uvm/stats.go:12:	"github.com/sirupsen/logrus"
internal/uvm/vpmem.go:11:	"github.com/sirupsen/logrus"
internal/uvm/vpmem_mapped.go:11:	"github.com/sirupsen/logrus"
internal/uvm/vsmb.go:14:	"github.com/sirupsen/logrus"
internal/uvm/wait.go:10:	"github.com/sirupsen/logrus"
internal/uvmfolder/locate.go:10:	"github.com/sirupsen/logrus"
internal/verity/verity.go:13:	"github.com/sirupsen/logrus"
internal/vhdx/info.go:18:	"github.com/sirupsen/logrus"
internal/vm/guestmanager/guest.go:16:	"github.com/sirupsen/logrus"
internal/vm/vmmanager/uvm.go:15:	"github.com/sirupsen/logrus"
internal/vm/vmutils/gcs_logs.go:16:	"github.com/sirupsen/logrus"
internal/vm/vmutils/gcs_logs_test.go:13:	"github.com/sirupsen/logrus"
internal/vm/vmutils/normalize.go:12:	"github.com/sirupsen/logrus"
internal/vm/vmutils/numa.go:9:	"github.com/sirupsen/logrus"
internal/vm/vmutils/utils.go:16:	"github.com/sirupsen/logrus"
internal/vmcompute/vmcompute.go:10:	"github.com/sirupsen/logrus"
internal/wclayer/cim/mount.go:16:	"github.com/sirupsen/logrus"
internal/wclayer/layerutils.go:17:	"github.com/sirupsen/logrus"
internal/winapi/cimfs.go:7:	"github.com/sirupsen/logrus"
internal/winapi/cimfs/cimfs.go:9:	"github.com/sirupsen/logrus"
internal/winapi/cimwriter/cimwriter.go:8:	"github.com/sirupsen/logrus"
internal/windevice/devicequery.go:16:	"github.com/sirupsen/logrus"
pkg/cimfs/cim_writer_windows.go:18:	"github.com/sirupsen/logrus"
pkg/cimfs/cimfs.go:13:	"github.com/sirupsen/logrus"
pkg/ociwclayer/cim/import.go:25:	"github.com/sirupsen/logrus"
pkg/securitypolicy/securitypolicy_options.go:22:	"github.com/sirupsen/logrus"
test/functional/main_test.go:25:	"github.com/sirupsen/logrus"
test/functional/uvm_scsi_test.go:16:	"github.com/sirupsen/logrus"
test/gcs/helper_conn_test.go:15:	"github.com/sirupsen/logrus"
test/gcs/main_test.go:15:	"github.com/sirupsen/logrus"
test/internal/layers/lazy.go:21:	"github.com/sirupsen/logrus"
test/internal/schemaversion_test.go:12:	"github.com/sirupsen/logrus"
test/pkg/flag/flag.go:8:	"github.com/sirupsen/logrus"
vendor/github.com/Microsoft/cosesign1go/pkg/cosesign1/check.go:9:	"github.com/sirupsen/logrus"
vendor/github.com/Microsoft/cosesign1go/pkg/cosesign1/create.go:11:	"github.com/sirupsen/logrus"
vendor/github.com/Microsoft/cosesign1go/pkg/cosesign1/makedidx509.go:14:	"github.com/sirupsen/logrus"
vendor/github.com/Microsoft/go-winio/pkg/etwlogrus/hook.go:9:	"github.com/sirupsen/logrus"
vendor/github.com/Microsoft/go-winio/pkg/etwlogrus/opts.go:6:	"github.com/sirupsen/logrus"
vendor/github.com/containerd/go-runc/io_unix.go:25:	"github.com/sirupsen/logrus"
vendor/github.com/containerd/log/context.go:44:	"github.com/sirupsen/logrus"
vendor/github.com/docker/cli/cli/config/configfile/file.go:16:	"github.com/sirupsen/logrus"
vendor/github.com/open-policy-agent/opa/logging/logging.go:8:	"github.com/sirupsen/logrus"
vendor/github.com/opencontainers/cgroups/file.go:13:	"github.com/sirupsen/logrus"
vendor/github.com/opencontainers/cgroups/fs/freezer.go:11:	"github.com/sirupsen/logrus"
vendor/github.com/opencontainers/cgroups/fs2/io.go:11:	"github.com/sirupsen/logrus"
vendor/github.com/opencontainers/cgroups/systemd/common.go:16:	"github.com/sirupsen/logrus"
vendor/github.com/opencontainers/cgroups/systemd/v1.go:11:	"github.com/sirupsen/logrus"
vendor/github.com/opencontainers/cgroups/systemd/v2.go:16:	"github.com/sirupsen/logrus"
vendor/github.com/opencontainers/cgroups/utils.go:17:	"github.com/sirupsen/logrus"
vendor/github.com/opencontainers/runc/internal/pathrs/mkdirall_pathrslite.go:27:	"github.com/sirupsen/logrus"
vendor/github.com/opencontainers/runc/libcontainer/capabilities/capabilities.go:15:	"github.com/sirupsen/logrus"
vendor/github.com/opencontainers/runc/libcontainer/configs/config.go:16:	"github.com/sirupsen/logrus"
vendor/github.com/opencontainers/runc/libcontainer/configs/validate/validator.go:16:	"github.com/sirupsen/logrus"
vendor/github.com/opencontainers/runc/libcontainer/container_linux.go:19:	"github.com/sirupsen/logrus"
vendor/github.com/opencontainers/runc/libcontainer/criu_linux.go:22:	"github.com/sirupsen/logrus"
vendor/github.com/opencontainers/runc/libcontainer/env.go:11:	"github.com/sirupsen/logrus"
vendor/github.com/opencontainers/runc/libcontainer/exeseal/cloned_binary_linux.go:10:	"github.com/sirupsen/logrus"
vendor/github.com/opencontainers/runc/libcontainer/init_linux.go:19:	"github.com/sirupsen/logrus"
vendor/github.com/opencontainers/runc/libcontainer/intelrdt/monitoring.go:9:	"github.com/sirupsen/logrus"
vendor/github.com/opencontainers/runc/libcontainer/internal/userns/userns_maps_linux.go:14:	"github.com/sirupsen/logrus"
vendor/github.com/opencontainers/runc/libcontainer/internal/userns/usernsfd_linux.go:11:	"github.com/sirupsen/logrus"
vendor/github.com/opencontainers/runc/libcontainer/logs/logs.go:8:	"github.com/sirupsen/logrus"
vendor/github.com/opencontainers/runc/libcontainer/mount_linux.go:11:	"github.com/sirupsen/logrus"
vendor/github.com/opencontainers/runc/libcontainer/notify_v2_linux.go:10:	"github.com/sirupsen/logrus"
vendor/github.com/opencontainers/runc/libcontainer/process_linux.go:19:	"github.com/sirupsen/logrus"
vendor/github.com/opencontainers/runc/libcontainer/rootfs_linux.go:22:	"github.com/sirupsen/logrus"
vendor/github.com/opencontainers/runc/libcontainer/seccomp/patchbpf/enosys_linux.go:16:	"github.com/sirupsen/logrus"
vendor/github.com/opencontainers/runc/libcontainer/seccomp/seccomp_linux.go:10:	"github.com/sirupsen/logrus"
vendor/github.com/opencontainers/runc/libcontainer/setns_init_linux.go:10:	"github.com/sirupsen/logrus"
vendor/github.com/opencontainers/runc/libcontainer/standard_init_linux.go:11:	"github.com/sirupsen/logrus"
vendor/github.com/opencontainers/runc/libcontainer/sync.go:13:	"github.com/sirupsen/logrus"
vendor/github.com/opencontainers/runc/libcontainer/system/linux.go:11:	"github.com/sirupsen/logrus"
vendor/github.com/opencontainers/runc/libcontainer/utils/utils_unix.go:17:	"github.com/sirupsen/logrus"
vendor/github.com/sirupsen/logrus/README.md:122:  log "github.com/sirupsen/logrus"
vendor/github.com/sirupsen/logrus/README.md:133:replace your `log` imports everywhere with `log "github.com/sirupsen/logrus"`
vendor/github.com/sirupsen/logrus/README.md:142:  log "github.com/sirupsen/logrus"
vendor/github.com/sirupsen/logrus/README.md:193:  "github.com/sirupsen/logrus"
vendor/github.com/sirupsen/logrus/README.md:268:  log "github.com/sirupsen/logrus"
vendor/github.com/sirupsen/logrus/README.md:343:  log "github.com/sirupsen/logrus"
vendor/github.com/sirupsen/logrus/README.md:465:  "github.com/sirupsen/logrus"
vendor/github.com/sirupsen/logrus/doc.go:10:    log "github.com/sirupsen/logrus"
```

## WithField usages

```txt
cmd/containerd-shim-lcow-v2/manager.go:211:		logrus.WithField("log", string(logBytes)).Warn("found shim panic logs during delete")
cmd/containerd-shim-lcow-v2/service/plugin/plugin.go:50:		etw.WithFields(
cmd/containerd-shim-lcow-v2/service/plugin/plugin.go:109:		log := logrus.WithField("sandboxID", svc.SandboxID())
cmd/containerd-shim-lcow-v2/service/plugin/plugin.go:110:		log.WithField("stack", resp.Stacks).Info("goroutine stack dump")
cmd/containerd-shim-lcow-v2/service/plugin/plugin.go:112:			log.WithField("stack", resp.GuestStacks).Info("guest stack dump")
cmd/containerd-shim-lcow-v2/service/service_sandbox.go:37:	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.SandboxID, request.SandboxID))
cmd/containerd-shim-lcow-v2/service/service_sandbox.go:53:	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.SandboxID, request.SandboxID))
cmd/containerd-shim-lcow-v2/service/service_sandbox.go:83:	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.SandboxID, request.SandboxID))
cmd/containerd-shim-lcow-v2/service/service_sandbox.go:99:	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.SandboxID, request.SandboxID))
cmd/containerd-shim-lcow-v2/service/service_sandbox.go:116:	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.SandboxID, request.SandboxID))
cmd/containerd-shim-lcow-v2/service/service_sandbox.go:132:	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.SandboxID, request.SandboxID))
cmd/containerd-shim-lcow-v2/service/service_sandbox.go:149:	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.SandboxID, request.SandboxID))
cmd/containerd-shim-lcow-v2/service/service_sandbox.go:166:	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.SandboxID, request.SandboxID))
cmd/containerd-shim-runhcs-v1/delete.go:80:			logrus.WithField("log", string(logBytes)).Warn("found shim panic logs during delete")
cmd/containerd-shim-runhcs-v1/exec_hcs.go:46:	log.G(ctx).WithFields(logrus.Fields{
cmd/containerd-shim-runhcs-v1/exec_hcs.go:205:		Log: log.G(ctx).WithFields(logrus.Fields{
cmd/containerd-shim-runhcs-v1/exec_hcs.go:309:							log.G(ctx).WithField("err", deliveryErr).Errorf("Error in delivering signal %d, to pid: %d", signal, he.pid)
cmd/containerd-shim-runhcs-v1/exec_hcs.go:409:		log.G(ctx).WithField("status", status).Debug("hcsExec::exitFromCreatedL")
cmd/containerd-shim-runhcs-v1/exec_hcs.go:471:		log.G(ctx).WithField("exitCode", code).Debug("exited")
cmd/containerd-shim-runhcs-v1/exec_wcow_podsandbox.go:22:	log.G(ctx).WithFields(logrus.Fields{
cmd/containerd-shim-runhcs-v1/exec_wcow_podsandbox.go:197:		log.G(ctx).WithField("status", status).Debug("wcowPodSandboxExec::ForceExit")
cmd/containerd-shim-runhcs-v1/main.go:63:		log := logrus.WithField("tid", svc.tid)
cmd/containerd-shim-runhcs-v1/main.go:64:		log.WithField("stack", resp.Stacks).Info("goroutine stack dump")
cmd/containerd-shim-runhcs-v1/main.go:66:			log.WithField("stack", resp.GuestStacks).Info("guest stack dump")
cmd/containerd-shim-runhcs-v1/main.go:98:		etw.WithFields(
cmd/containerd-shim-runhcs-v1/pod.go:40:	log.G(ctx).WithField("options", log.Format(ctx, *wopts)).Debug("initialize WCOW boot files")
cmd/containerd-shim-runhcs-v1/pod.go:136:	log.G(ctx).WithField("tid", req.ID).Debug("createPod")
cmd/containerd-shim-runhcs-v1/serve.go:324:	logrus.WithField("event", event).Info("Halting until signalled")
cmd/containerd-shim-runhcs-v1/service_internal.go:90:			entry.WithField("options", log.Format(ctx, shimOpts)).Debug("parsed runhcs runtime options")
cmd/containerd-shim-runhcs-v1/task_hcs.go:56:	log.G(ctx).WithField("tid", req.ID).Debug("newHcsStandaloneTask")
cmd/containerd-shim-runhcs-v1/task_hcs.go:191:	log.G(ctx).WithFields(logrus.Fields{
cmd/containerd-shim-runhcs-v1/task_hcs.go:436:				log.G(ctx).WithFields(logrus.Fields{
cmd/containerd-shim-runhcs-v1/task_wcow_podsandbox.go:39:	log.G(ctx).WithField("tid", id).Debug("newWcowPodSandboxTask")
cmd/gcs-sidecar/main.go:218:	logrus.WithFields(logrus.Fields{
cmd/gcs/main.go:58:			logrus.WithError(err).WithField("cgroup", cgName).Error("failed to read from eventfd")
cmd/gcs/main.go:77:		entry := logrus.WithFields(logrus.Fields{
cmd/gcs/main.go:94:			entry.WithFields(memoryLogFormat(metrics)).Warn(msg)
cmd/gcs/main.go:111:			logrus.WithFields(logrus.Fields{
cmd/gcs/main.go:231:			logrus.WithFields(logrus.Fields{
cmd/gcs/main.go:252:		logrus.WithFields(logrus.Fields{
cmd/gcs/main.go:265:		logrus.WithFields(logrus.Fields{
cmd/gcs/main.go:281:	logrus.WithFields(logrus.Fields{
cmd/gcs/main.go:380:			logrus.WithFields(logrus.Fields{
cmd/gcs/main.go:424:		logrus.WithFields(logrus.Fields{
cmd/gcstools/generichook.go:72:		logrus.WithField("output", output).Infof("%s debug part %d", debugFilePath, i)
cmd/ncproxy/hcn.go:309:		log.G(ctx).WithField("networkName", network.Name).Warn("network has multiple MAC pools, only returning the first")
cmd/ncproxy/ncproxy.go:125:			log.G(ctx).WithField("iov settings", settings.Policies.IovPolicySettings).Info("AddNIC iov settings")
cmd/ncproxy/ncproxy.go:201:	log.G(ctx).WithField("iov settings", settings.Policies.IovPolicySettings).Info("ModifyNIC iov settings")
cmd/ncproxy/ncproxy.go:470:				log.G(ctx).WithField("namespaceID", req.NamespaceID).
cmd/ncproxy/ncproxy.go:480:			log.G(ctx).WithField("namespaceID", req.NamespaceID).Debug("Attaching endpoint to default host namespace")
cmd/ncproxy/ncproxy.go:812:		log.G(ctx).WithField("key", req.ContainerID).WithError(err).Warn("failed to delete key from compute agent store")
cmd/ncproxy/run.go:90:		logrus.WithField("stack", stacks).Info("ncproxy goroutine stack dump")
cmd/ncproxy/run.go:277:	log.G(ctx).WithFields(logrus.Fields{
cmd/ncproxy/server.go:122:		log.G(ctx).WithFields(logrus.Fields{
cmd/ncproxy/server.go:131:		log.G(ctx).WithFields(logrus.Fields{
cmd/ncproxy/server.go:184:				log.G(ctx).WithField("agentAddress", agentAddress).WithError(err).Error("failed to create new compute agent client")
cmd/ncproxy/server.go:187:					log.G(ctx).WithField("key", containerID).WithError(dErr).Warn("failed to delete key from compute agent store")
cmd/ncproxy/server.go:191:			log.G(ctx).WithField("containerID", containerID).Info("reconnected to container's compute agent")
cmd/runhcs/container.go:493:			logrus.WithFields(logrus.Fields{
cmd/runhcs/container.go:524:	logrus.WithFields(logrus.Fields{
cmd/runhcs/vm.go:152:	logrus.WithFields(logrus.Fields{
hcn/hcnnamespace.go:360:	logrus.WithField("id", namespace.Id).Debugf("hcs::HostComputeNamespace::Sync")
hcn/hcnnamespace.go:392:		logrus.WithFields(f).
hcn/hcnsupport.go:124:	log.L.WithFields(logrus.Fields{
internal/bridgeutils/commonutils/utilities.go:59:			logrus.WithFields(logrus.Fields{
internal/builder/vm/lcow/boot.go:38:		log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/boot.go:48:	log.G(ctx).WithField(logfields.Path, bootFilesRootPath).Debug("resolveBootFilesPath completed successfully")
internal/builder/vm/lcow/boot.go:63:	log.G(ctx).WithField(logfields.Path, bootFilesPath).Debug("using boot files path")
internal/builder/vm/lcow/boot.go:80:		log.G(ctx).WithField(
internal/builder/vm/lcow/boot.go:92:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/boot.go:108:			log.G(ctx).WithField(vmutils.UncompressedKernelFile, filepath.Join(bootFilesPath, vmutils.UncompressedKernelFile)).Debug("updated LCOW kernel file to " + vmutils.UncompressedKernelFile)
internal/builder/vm/lcow/boot.go:121:	log.G(ctx).WithField("kernelFile", kernelFileName).Debug("selected kernel file")
internal/builder/vm/lcow/boot.go:125:		log.G(ctx).WithField("preferredRootFSType", preferredRootfsType).Debug("applying preferred rootfs type override")
internal/builder/vm/lcow/boot.go:139:	log.G(ctx).WithField("rootFsFile", rootFsFile).Debug("selected rootfs file")
internal/builder/vm/lcow/boot.go:150:			log.G(ctx).WithField("initrdPath", chipset.LinuxKernelDirect.InitRdPath).Debug("configured initrd for kernel direct boot")
internal/builder/vm/lcow/boot.go:165:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/confidential.go:65:	log.G(ctx).WithField("vmgsTemplatePath", vmgsTemplatePath).Debug("VMGS template path configured")
internal/builder/vm/lcow/confidential.go:81:	log.G(ctx).WithField("dmVerityRootfsPath", dmVerityRootfsTemplatePath).Debug("DM Verity rootfs path configured")
internal/builder/vm/lcow/confidential.go:141:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/confidential.go:177:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/confidential.go:189:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/confidential.go:228:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/devices.go:38:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/devices.go:61:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/devices.go:96:			log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/devices.go:112:	log.G(ctx).WithField("scsiControllerCount", scsiControllerCount).Debug("configuring SCSI controllers")
internal/builder/vm/lcow/devices.go:131:		log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/devices.go:187:				log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/devices.go:204:		log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/devices.go:215:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/kernel_args.go:32:	log.G(ctx).WithField("rootFsFile", rootFsFile).Debug("buildKernelArgs: starting kernel arguments construction")
internal/builder/vm/lcow/kernel_args.go:88:	log.G(ctx).WithField("kernelArgs", result).Debug("buildKernelArgs completed successfully")
internal/builder/vm/lcow/kernel_args.go:100:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/kernel_args.go:161:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/specs.go:54:	log.G(ctx).WithField("platform", platform).Debug("validating sandbox platform")
internal/builder/vm/lcow/specs.go:226:		log.G(ctx).WithField("kernelArgs", kernelArgs).Debug("kernel arguments configured")
internal/builder/vm/lcow/specs.go:279:	log.G(ctx).WithField("annotationCount", len(annotations)).Debug("processAnnotations: starting annotations processing")
internal/builder/vm/lcow/specs.go:308:	log.G(ctx).WithField("platform", platform).Debug("parseSandboxOptions: starting sandbox options parsing")
internal/builder/vm/lcow/specs.go:338:		log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/specs.go:359:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/topology.go:62:		log.G(ctx).WithField("resourcePartitionID", resourcePartitionID).Debug("setting resource partition ID")
internal/builder/vm/lcow/topology.go:74:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/topology.go:116:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/topology.go:149:		log.G(ctx).WithField("virtualNodeCount", hcsNuma.VirtualNodeCount).Debug("vNUMA topology configured")
internal/builder/vm/lcow/topology.go:158:	log.G(ctx).WithFields(logrus.Fields{
internal/cmd/cmd.go:209:		c.Log = c.Log.WithField("pid", p.Pid())
internal/cmd/cmd.go:306:					c.Log.WithField("timeout", c.CopyAfterExitTimeout).Warn(err.Error())
internal/cmd/io.go:71:		log = log.WithFields(logrus.Fields{
internal/cmd/io_binary.go:110:	log.G(ctx).WithFields(logrus.Fields{
internal/cmd/io_npipe.go:26:	log.G(ctx).WithFields(logrus.Fields{
internal/cmd/io_npipe.go:111:				log.G(nprw.ctx).WithFields(logrus.Fields{
internal/cmd/io_npipe.go:120:					log.G(nprw.ctx).WithField("address", nprw.pipePath).Info("Succeeded in reconnecting to named pipe")
internal/computecore/computecore.go:137:		log.G(ctx).WithFields(logrus.Fields{
internal/computecore/computecore.go:150:			log.G(ctx).WithField(logfields.Timeout, trueTimeout).
internal/controller/vm/vm.go:87:	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.Operation, "CreateVM"))
internal/controller/vm/vm.go:123:	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.Operation, "StartVM"))
internal/controller/vm/vm.go:220:		log.G(ctx).WithField("currentState", c.vmState).Debug("waitForVMExit: state transition to Terminated was a no-op")
internal/controller/vm/vm.go:227:	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.Operation, "ExecIntoHost"))
internal/controller/vm/vm.go:260:	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.Operation, "DumpStacks"))
internal/controller/vm/vm.go:282:	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.Operation, "Wait"))
internal/controller/vm/vm.go:312:	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.Operation, "Stats"))
internal/controller/vm/vm.go:380:	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.Operation, "TerminateVM"))
internal/credentials/credentials.go:51:	log.G(ctx).WithField("containerID", id).Debug("creating container credential guard instance")
internal/credentials/credentials.go:118:	log.G(ctx).WithField("containerID", id).Debug("removing container credential guard")
internal/devices/assigned_devices.go:49:		log.G(ctx).WithField("vmbus id", vmBusInstanceID).Info("vmbus instance ID")
internal/devices/pnp.go:60:		log.G(ctx).WithFields(logrus.Fields{
internal/devices/pnp.go:67:	log.G(ctx).WithField("added drivers", driverDir).Debug("installed drivers")
internal/gcs-sidecar/bridge.go:141:		logrus.WithFields(logrus.Fields{
internal/gcs-sidecar/handlers.go:97:						log.G(ctx).WithField("name", value.Name).Trace("Registry value matches default, accepting without policy check")
internal/gcs-sidecar/handlers.go:729:							log.G(ctx).WithFields(map[string]interface{}{
internal/gcs-sidecar/host.go:108:	logrus.WithFields(logrus.Fields{
internal/gcs/bridge.go:230:		brdg.log.WithField("reason", ctx.Err()).Warn("ignoring response to bridge message")
internal/gcs/bridge.go:296:		brdg.log.WithFields(logrus.Fields{
internal/gcs/bridge.go:316:					brdg.log.WithFields(logrus.Fields{
internal/gcs/bridge.go:406:		brdg.log.WithFields(logrus.Fields{
internal/gcs/guestconnection.go:290:	logrus.WithField(logfields.ContainerID, cid).Info("container terminated in guest")
internal/gcs/process.go:110:	log.G(ctx).WithField("pid", p.id).Debug("created process pid")
internal/gcs/process.go:257:			log.G(ctx).WithFields(logrus.Fields{
internal/gcs/process.go:292:	log.G(ctx).WithField("exitCode", ec).Debug("process exited")
internal/guest/bridge/bridge.go:83:		logrus.WithFields(logrus.Fields{
internal/guest/bridge/bridge.go:327:					entry.WithField("message", s).Trace("request read message")
internal/guest/bridge/bridge.go:391:				log.G(resp.ctx).WithField("message", string(responseBytes)).Trace("request write response")
internal/guest/bridge/bridge_v2.go:202:	log.G(ctx).WithField("pid", pid).Debug("created process pid")
internal/guest/kmsg/kmsg.go:135:			logrus.WithFields(logrus.Fields{
internal/guest/kmsg/kmsg.go:141:				logrus.WithFields(entry.logFormat()).Info("kmsg read")
internal/guest/network/netns.go:102:	entry.WithField("namespace", ns).Debug("New network namespace from PID")
internal/guest/network/netns.go:114:		entry.WithField("mtu", mtu).Debug("EncapOverhead non-zero, will set MTU")
internal/guest/network/netns.go:134:		entry.WithField("timeout", timeout.String()).Debug("Execing udhcpc with timeout...")
internal/guest/network/netns.go:151:			entry.WithField("timeout", timeout.String()).Warningf("udhcpc timed out [%s]", cos)
internal/guest/network/netns.go:184:		entry.WithField("addresses", addrsStr).Debugf("%v: %s[idx=%d,type=%s] is %v",
internal/guest/network/netns.go:197:		log.G(ctx).WithFields(logrus.Fields{
internal/guest/network/netns.go:210:		log.G(ctx).WithFields(logrus.Fields{
internal/guest/network/netns.go:240:		log.G(ctx).WithField("route", r).Debugf("adding a route to interface %s", link.Attrs().Name)
internal/guest/network/network.go:142:	log.G(ctx).WithField("ifname", ifname).Debug("resolved ifname")
internal/guest/runtime/hcsv2/container.go:83:	entity := log.G(ctx).WithField(logfields.ContainerID, c.id)
internal/guest/runtime/hcsv2/container.go:102:				entity.WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/container.go:136:	log.G(ctx).WithField(logfields.ContainerID, c.id).Info("opengcs::Container::ExecProcess")
internal/guest/runtime/hcsv2/container.go:183:	logrus.WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/container.go:203:	log.G(ctx).WithField(logfields.ContainerID, c.id).Info("opengcs::Container::GetAllProcessPids")
internal/guest/runtime/hcsv2/container.go:217:	log.G(ctx).WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/container.go:230:	entity := log.G(ctx).WithField(logfields.ContainerID, c.id)
internal/guest/runtime/hcsv2/container.go:280:	log.G(ctx).WithField(logfields.ContainerID, c.id).Info("opengcs::Container::Update")
internal/guest/runtime/hcsv2/nvidia_utils.go:75:		log.G(ctx).WithField("hook", log.Format(ctx, nvidiaHook)).Debug("adding nvidia device runtime hook")
internal/guest/runtime/hcsv2/process.go:102:		log.G(ctx).WithField("exitCode", p.exitCode).Debug("process exited")
internal/guest/runtime/hcsv2/process.go:113:				log.G(ctx).WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/process.go:244:		log.G(ctx).WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/uvm.go:161:		logrus.WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/uvm.go:335:		logrus.WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/uvm.go:365:		logrus.WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/uvm.go:569:		log.G(ctx).WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/uvm.go:660:		entry := log.G(ctx).WithField(logfields.Path, configFile)
internal/guest/runtime/hcsv2/uvm.go:665:			log.G(ctx).WithField(
internal/guest/runtime/hcsv2/uvm.go:986:				log.G(ctx).WithField("stats", log.Format(ctx, cgroupMetrics)).Trace("queried cgroup statistics")
internal/guest/runtime/hcsv2/uvm.go:990:			log.G(ctx).WithField("propertyType", requestedProperty).Warn("unknown or empty property type")
internal/guest/runtime/hcsv2/uvm.go:1461:		logrus.WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/uvm.go:1465:		logrus.WithField("virtualSandboxID", virtualSandboxID).Info("Creating pod cgroup with default resources as none were specified")
internal/guest/runtime/hcsv2/uvm.go:1486:	logrus.WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/uvm.go:1525:	logrus.WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/uvm.go:1550:				logrus.WithError(err).WithField("virtualSandboxID", virtualSandboxID).
internal/guest/runtime/hcsv2/uvm.go:1563:	logrus.WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/uvm.go:1574:			logrus.WithError(err).WithField("virtualSandboxID", virtualSandboxID).
internal/guest/runtime/hcsv2/uvm.go:1584:				logrus.WithError(err).WithField("virtualSandboxID", virtualSandboxID).
internal/guest/runtime/hcsv2/uvm.go:1591:		logrus.WithField("virtualSandboxID", virtualSandboxID).Info("Virtual pod cleaned up")
internal/guest/runtime/runc/container.go:84:	logrus.WithFields(logrus.Fields{
internal/guest/runtime/runc/container.go:103:	logrus.WithField(logfields.ContainerID, c.id).Debug("runc::container::killAll")
internal/guest/runtime/runc/container.go:125:	logrus.WithField(logfields.ContainerID, c.id).Debug("runc::container::Delete")
internal/guest/runtime/runc/container.go:248:	logrus.WithFields(logrus.Fields{
internal/guest/runtime/runc/container.go:318:	entity := logrus.WithField(logfields.ContainerID, c.id)
internal/guest/runtime/runc/container.go:330:			entity.WithField(logfields.ProcessID, process.Pid).Debug("waiting on container exec process")
internal/guest/runtime/runc/process.go:48:	l := logrus.WithField(logfields.ContainerID, p.c.id)
internal/guest/runtime/runc/process.go:49:	l.WithField(logfields.ContainerID, p.pid).Debug("process wait completed")
internal/guest/runtime/runc/process.go:89:	l.WithField(logfields.ProcessID, p.pid).Debug("relay wait completed")
internal/guest/spec/spec.go:52:	logrus.WithFields(logrus.Fields{
internal/guest/spec/spec.go:467:		log.G(ctx).WithField("sizeKB", val).Debug("set custom /dev/shm size")
internal/guest/spec/spec.go:475:			log.G(ctx).WithFields(logrus.Fields{
internal/guest/spec/spec_devices.go:52:		entry := log.G(ctx).WithField("windows-device", log.Format(ctx, d))
internal/guest/spec/spec_devices.go:63:			entry.WithField("path", fullPCIPath).Trace("found PCI path for Windows device")
internal/guest/spec/spec_devices.go:71:				entry.WithFields(logrus.Fields{
internal/guest/spec/spec_devices.go:116:			entry := log.G(ctx).WithField("host-device", log.Format(ctx, d))
internal/guest/stdio/stdio.go:171:		logrus.WithFields(logrus.Fields{
internal/guest/stdio/stdio.go:187:			logrus.WithFields(logrus.Fields{
internal/guest/stdio/stdio.go:193:		logrus.WithFields(logrus.Fields{
internal/guest/stdio/stdio.go:199:		logrus.WithFields(logrus.Fields{
internal/guest/stdio/stdio.go:213:				logrus.WithFields(logrus.Fields{
internal/guest/stdio/stdio.go:219:				logrus.WithFields(logrus.Fields{
internal/guest/stdio/stdio.go:289:					logrus.WithFields(logrus.Fields{
internal/guest/stdio/stdio.go:337:				logrus.WithFields(logrus.Fields{
internal/guest/stdio/stdio.go:348:				logrus.WithFields(logrus.Fields{
internal/guest/storage/crypt/crypt.go:51:			log.G(ctx).WithError(err).WithFields(logrus.Fields{
internal/guest/storage/overlay/overlay.go:36:		log.G(ctx).WithError(statErr).WithField("path", filepath.Dir(path)).Warn("failed to get disk information for ENOSPC error")
internal/guest/storage/overlay/overlay.go:49:	log.G(ctx).WithFields(logrus.Fields{
internal/guest/storage/scsi/scsi.go:163:						log.G(spnCtx).WithError(err).WithField("verityTarget", dmVerityName).Debug("failed to cleanup verity target")
internal/guest/storage/scsi/scsi.go:176:		log.G(ctx).WithFields(logrus.Fields{
internal/guest/storage/scsi/scsi.go:230:		log.G(ctx).WithField("filesystem", deviceFS).Debug("filesystem found on device")
internal/guest/storage/scsi/scsi.go:302:		log.G(ctx).WithField("target", target).Trace("removing block device symlink")
internal/guest/storage/scsi/scsi.go:365:					log.G(ctx).WithField("blockPath", blockPath).Warn(
internal/guest/storage/scsi/scsi.go:412:	log.G(ctx).WithField("devicePath", devicePath).Debug("found device path")
internal/guest/transport/devnull.go:23:	logrus.WithFields(logrus.Fields{
internal/guest/transport/log.go:19:	return &logConnection{c, logrus.WithField("port", port)}
internal/guest/transport/vsock.go:26:	logrus.WithFields(logrus.Fields{
internal/hcs/callback.go:149:	log := logrus.WithFields(logrus.Fields{
internal/hcs/process.go:150:		log.G(ctx).WithField("err", err).Error("OpenComputeSystem() call failed")
internal/hcs/process.go:154:			log.G(ctx).WithField("err", err).Error("Terminate() call failed")
internal/hcs/process.go:248:						log.G(ctx).WithField("wait-result", properties.LastWaitResult).Warning("non-zero last wait result")
internal/hcs/process.go:256:	log.G(ctx).WithField("exitCode", exitCode).Debug("process exited")
internal/hcs/system.go:553:	logEntry.WithFields(logrus.Fields{
internal/hcs/system.go:691:	log.G(ctx).WithField("pid", processInfo.ProcessId).Debug("created process pid")
internal/hcs/waithelper.go:37:		log.G(ctx).WithField("callbackNumber", callbackNumber).Error("failed to waitForNotification: callbackNumber does not exist in callbackMap")
internal/hcs/waithelper.go:45:		log.G(ctx).WithField("type", expectedNotification).Error("unknown notification type in waitForNotification")
internal/hcsoci/create.go:131:	log.G(ctx).WithFields(logrus.Fields{
internal/hcsoci/devices.go:144:			log.G(ctx).WithField("parsed devices", specDev).Info("added windows device to spec")
internal/hcsoci/hcsdoc_lcow.go:110:	log.G(ctx).WithField("guestRoot", guestRoot).Debug("hcsshim::createLinuxContainerDoc")
internal/hcsoci/hcsdoc_wcow.go:222:		log.G(ctx).WithFields(logrus.Fields{
internal/hcsoci/hcsdoc_wcow.go:246:			log.G(ctx).WithFields(logrus.Fields{
internal/hcsoci/hcsdoc_wcow.go:493:		log.G(ctx).WithField("count", len(testAnnotationValues)).Info("adding test annotation registry values to container")
internal/hcsoci/hcsdoc_wcow.go:526:		log.G(ctx).WithField("hcsv2 device", v2Dev).Debug("adding assigned device to container doc")
internal/hcsoci/network.go:18:	l := log.G(ctx).WithField(logfields.ContainerID, coi.ID)
internal/hcsoci/network.go:29:	log.G(ctx).WithFields(logrus.Fields{
internal/hcsoci/network.go:43:		log.G(ctx).WithFields(logrus.Fields{
internal/hcsoci/resources.go:25:		log.G(ctx).WithFields(logrus.Fields{
internal/hcsoci/resources.go:40:		log.G(ctx).WithFields(logrus.Fields{
internal/hcsoci/resources_lcow.go:86:			l := log.G(ctx).WithField("mount", fmt.Sprintf("%+v", mount))
internal/hcsoci/resources_wcow.go:149:		l := log.G(ctx).WithField("mount", fmt.Sprintf("%+v", mount))
internal/jobcontainers/jobcontainer.go:102:	log.G(ctx).WithField("id", id).Debug("Creating job container")
internal/jobcontainers/jobcontainer.go:441:		log.G(context.Background()).WithError(err).WithField("cid", c.id).Warning("failed to close job object")
internal/jobcontainers/jobcontainer.go:446:		log.G(context.Background()).WithError(err).WithField("cid", c.id).Warning("failed to close token")
internal/jobcontainers/jobcontainer.go:453:			log.G(context.Background()).WithError(err).WithField("cid", c.id).Warning("failed to delete local account")
internal/jobcontainers/jobcontainer.go:475:	log.G(ctx).WithField("id", c.id).Debug("shutting down job container")
internal/jobcontainers/jobcontainer.go:499:			log.G(ctx).WithField("pid", pid).Error("failed to signal process in job container")
internal/jobcontainers/jobcontainer.go:604:	log.G(ctx).WithField("id", c.id).Debug("terminating job container")
internal/jobcontainers/jobcontainer.go:667:			log.G(ctx).WithField("message", msg).Warn("unknown job object notification encountered")
internal/jobcontainers/logon.go:77:	log.G(ctx).WithField("username", user).Debug("Created local user account for job container")
internal/jobcontainers/mounts.go:104:			log.G(ctx).WithFields(logrus.Fields{
internal/jobcontainers/process.go:161:	log.G(ctx).WithField("pid", p.Pid()).Debug("waitBackground for JobProcess")
internal/jobcontainers/process.go:231:	log.G(ctx).WithField("pid", p.Pid()).Debug("killing job process")
internal/jobobject/iocp.go:52:				log.G(ctx).WithField("value", msq).Warn("encountered non queue type in job map")
internal/jobobject/iocp.go:57:				log.G(ctx).WithFields(logrus.Fields{
internal/jobobject/iocp.go:69:				log.G(ctx).WithFields(logrus.Fields{
internal/layers/lcow.go:64:			log.G(ctx).WithFields(logrus.Fields{
internal/layers/lcow.go:97:	log.G(ctx).WithField("os", vm.OS()).Debug("hcsshim::MountLCOWLayers V2 UVM")
internal/layers/lcow.go:114:		log.G(ctx).WithField("layerPath", layer.VHDPath).Debug("mounting layer")
internal/layers/lcow.go:134:	log.G(ctx).WithField("hostPath", hostPath).Debug("mounting scratch VHD")
internal/layers/lcow.go:198:			log.G(ctx).WithFields(logrus.Fields{
internal/layers/lcow.go:222:	log.G(ctx).WithFields(logrus.Fields{
internal/layers/wcow_mount.go:165:					log.G(ctx).WithField("path", l.scratchLayerPath).WithError(hcserr.Err).Warning("retrying layer operations after failure")
internal/layers/wcow_mount.go:252:	log.G(ctx).WithFields(logrus.Fields{
internal/layers/wcow_mount.go:281:	log.G(ctx).WithField("layer data", layerData).Debug("unionFS filter attached")
internal/layers/wcow_mount.go:341:	log.G(ctx).WithFields(logrus.Fields{
internal/layers/wcow_mount.go:359:	log.G(ctx).WithField("volume", volume).Debug("mounted blockCIM layers for process isolated container")
internal/layers/wcow_mount.go:392:			log.G(ctx).WithFields(logrus.Fields{
internal/layers/wcow_mount.go:405:	log.G(ctx).WithField("os", vm.OS()).Debug("hcsshim::MountWCOWLayers V2 UVM")
internal/layers/wcow_mount.go:428:		log.G(ctx).WithField("layerPath", layerPath).Debug("mounting layer")
internal/layers/wcow_mount.go:440:	log.G(ctx).WithField("hostPath", hostPath).Debug("mounting scratch VHD")
internal/layers/wcow_mount.go:512:	log.G(ctx).WithFields(logrus.Fields{
internal/layers/wcow_mount.go:525:	log.G(ctx).WithField("volume", mountedCIMs.MountedVolumePath()).Debug("mounted blockCIM layers for hyperV isolated container")
internal/layers/wcow_mount.go:539:	log.G(ctx).WithFields(logrus.Fields{
internal/layers/wcow_mount.go:582:	log.G(ctx).WithFields(logrus.Fields{
internal/layers/wcow_mount.go:612:	log.G(ctx).WithFields(logrus.Fields{
internal/lcow/common.go:48:	log.G(ctx).WithFields(logrus.Fields{
internal/lcow/common.go:64:	log.G(ctx).WithField("dest", destPath).Debug("lcow::FormatDisk complete")
internal/lcow/disk.go:29:	log.G(ctx).WithFields(logrus.Fields{
internal/lcow/disk.go:43:	log.G(ctx).WithFields(logrus.Fields{
internal/lcow/disk.go:52:	log.G(ctx).WithField("dest", destPath).Debug("lcow::FormatDisk complete")
internal/lcow/scratch.go:47:	log.G(ctx).WithFields(logrus.Fields{
internal/lcow/scratch.go:59:			log.G(ctx).WithFields(logrus.Fields{
internal/lcow/scratch.go:93:	log.G(ctx).WithFields(logrus.Fields{
internal/lcow/scratch.go:108:		log.G(ctx).WithError(err).WithField("stderr", mkfsStderr.String()).Error("mkfs.ext4 failed")
internal/lcow/scratch.go:125:	log.G(ctx).WithField("dest", destFile).Debug("lcow::CreateScratch created (non-cache)")
internal/log/context.go:14:	// L is the default, blank logging entry. WithField and co. all return a copy
internal/log/context.go:52://	entry := GetEntry(ctx).WithFields(fields)
internal/log/context.go:59:		e = e.WithFields(fields)
internal/log/format.go:69:		G(ctx).WithFields(logrus.Fields{
internal/oc/exporter.go:34:		logrus.WithFields(logrus.Fields{
internal/oc/exporter.go:47:	// can skip overhead in entry.WithFields() and add them directly to entry.Data.
internal/oci/annotations.go:46:					log.G(ctx).WithFields(logrus.Fields{
internal/oci/annotations.go:69:		log.G(ctx).WithFields(logrus.Fields{
internal/oci/annotations.go:118:		entry := log.G(ctx).WithFields(logrus.Fields{
internal/oci/annotations.go:230:		entry := log.G(ctx).WithFields(logrus.Fields{
internal/oci/annotations.go:250:			entry.WithFields(logrus.Fields{
internal/oci/annotations.go:256:			entry.WithField("configuration", log.Format(ctx, conf)).Trace("found Hyper-V socket service configuration annotation")
internal/oci/annotations.go:405:	entry := log.G(ctx).WithFields(logrus.Fields{
internal/oci/uvm.go:140:			log.G(ctx).WithFields(logrus.Fields{
internal/oci/uvm.go:294:			log.G(ctx).WithFields(logrus.Fields{
internal/schemaversion/schemaversion.go:98:			logrus.WithField("schemaVersion", requestedSV).Warn("Ignoring unsupported requested schema version")
internal/shim/shim.go:198:	ctx = log.WithLogger(ctx, log.G(ctx).WithField("runtime", manager.Name()))
internal/shim/shim.go:261:		logger := log.G(ctx).WithFields(log.Fields{
internal/shim/shim.go:354:		log.G(ctx).WithFields(log.Fields{"id": pID, "type": p.Type}).Debug("loading plugin")
internal/shim/shim.go:388:				log.G(ctx).WithFields(log.Fields{"id": pID, "type": p.Type, "error": err}).Info("skip loading plugin")
internal/shim/shim.go:395:			log.G(ctx).WithField("id", pID).Debug("registering ttrpc service")
internal/shim/shim.go:485:	logger := log.G(ctx).WithFields(log.Fields{
internal/shim/shim_windows.go:115:	log.L.WithField("pipe", path).Debug("serving api on named pipe")
internal/shim/shim_windows.go:129:			logger.WithField("signal", s).Debug("received signal in reap loop")
internal/shim/shim_windows.go:146:			logger.WithField("signal", s).Debug("caught exit signal")
internal/tools/networkagent/main.go:144:	log.G(ctx).WithField("endpt", endpt).Info("ConfigureContainerNetworking created endpoint")
internal/tools/networkagent/main.go:225:	log.G(ctx).WithField("endpt", endpt).Info("ConfigureContainerNetworking created endpoint")
internal/tools/networkagent/main.go:268:			log.G(ctx).WithField("name", endpoint.ID).Warn("failed to get endpoint settings")
internal/tools/networkagent/main.go:279:				log.G(ctx).WithField("name", endpoint.ID).Warn("invalid endpoint settings type")
internal/tools/networkagent/main.go:286:				log.G(ctx).WithField("name", endpointName).Warn("failed to delete endpoint")
internal/tools/networkagent/main.go:297:				log.G(ctx).WithField("name", networkName).Warn("failed to delete network")
internal/tools/networkagent/main.go:308:	log.G(ctx).WithField("req", req).Info("ConfigureContainerNetworking request")
internal/tools/networkagent/main.go:342:	log.G(ctx).WithField("endpts", resp.Endpoints).Info("ConfigureNetworking addrequest")
internal/tools/networkagent/main.go:350:			log.G(ctx).WithField("name", endpoint.ID).Warn("failed to get endpoint settings")
internal/tools/networkagent/main.go:366:				log.G(ctx).WithField("name", endpoint.ID).Warn("invalid endpoint settings type")
internal/tools/networkagent/main.go:403:			log.G(ctx).WithField("name", endpoint.ID).Warn("failed to get endpoint settings")
internal/tools/networkagent/main.go:415:				log.G(ctx).WithField("name", endpoint.ID).Warn("invalid endpoint settings type")
internal/tools/networkagent/main.go:420:				log.G(ctx).WithField("name", endpointName).Warn("endpoint was not assigned a NIC ID previously")
internal/tools/networkagent/main.go:430:				log.G(ctx).WithField("name", endpointName).Warn("failed to delete endpoint nic")
internal/tools/networkagent/main.go:439:	log.G(ctx).WithField("req", req).Info("ConfigureNetworking request")
internal/tools/networkagent/main.go:470:	log.G(ctx).WithFields(logrus.Fields{
internal/tools/networkagent/main.go:492:	log.G(ctx).WithField("addr", conf.GRPCAddr).Info("connected to ncproxy")
internal/tools/networkagent/main.go:523:	log.G(ctx).WithField("addr", conf.NodeNetSvcAddr).Info("serving network service agent")
internal/tools/networkagent/v0_service_wrapper.go:53:	log.G(ctx).WithField("req", req).Info("ConfigureNetworking request")
internal/tools/rootfs/main.go:70:				logrus.WithFields(logrus.Fields{
internal/tools/rootfs/merge.go:141:	logrus.WithFields(logrus.Fields{
internal/tools/rootfs/merge.go:178:	logrus.WithFields(logrus.Fields{
internal/tools/rootfs/merge.go:204:		logrus.WithFields(logrus.Fields{
internal/tools/rootfs/merge.go:259:		entry := logrus.WithFields(logrus.Fields{
internal/tools/rootfs/merge.go:321:			entry.WithFields(logrus.Fields{
internal/tools/rootfs/merge.go:336:			entry.WithFields(logrus.Fields{
internal/tools/rootfs/merge.go:393:	entry := logrus.WithField("output", p)
internal/tools/uvmboot/lcow.go:292:		entry := log.G(ctx).WithField("flag-value", s)
internal/tools/uvmboot/lcow.go:296:			entry.WithField(logrus.ErrorKey, "missing `=` in annotation").Warnf("invald %s flag value", name)
internal/tools/uvmboot/lcow.go:298:			entry.WithField(logrus.ErrorKey, "empty annotation key or value").Warnf("invald %s flag value", name)
internal/tools/uvmboot/lcow.go:300:			entry = entry.WithFields(logrus.Fields{
internal/tools/uvmboot/lcow.go:307:				entry.WithField(logfields.Value+"-existing", vv).Warn("overriding existing annotation")
internal/tools/uvmboot/main.go:160:					logrus.WithField("uvm-id", id).WithError(err).Error("failed to run UVM")
internal/tools/uvmboot/mounts.go:34:		log.G(ctx).WithFields(logrus.Fields{
internal/tools/uvmboot/mounts.go:62:		log.G(ctx).WithFields(logrus.Fields{
internal/tools/uvmboot/mounts.go:99:		entry := log.G(ctx).WithField("flag-value", s)
internal/uvm/cimfs.go:121:					log.G(ctx).WithFields(logrus.Fields{
internal/uvm/cimfs.go:140:		log.G(ctx).WithFields(logrus.Fields{
internal/uvm/computeagent.go:66:	log.G(ctx).WithFields(logrus.Fields{
internal/uvm/computeagent.go:84:	log.G(ctx).WithFields(logrus.Fields{
internal/uvm/computeagent.go:100:	log.G(ctx).WithFields(logrus.Fields{
internal/uvm/computeagent.go:166:	log.G(ctx).WithFields(logrus.Fields{
internal/uvm/computeagent.go:216:	log.G(ctx).WithFields(logrus.Fields{
internal/uvm/computeagent.go:266:	log.G(ctx).WithField("address", l.Addr().String()).Info("serving compute agent")
internal/uvm/create.go:282:	log.G(ctx).WithFields(logrus.Fields{
internal/uvm/create.go:337:		e := log.G(ctx).WithField("VMGS file", vmgsFullPath)
internal/uvm/create_lcow.go:159:		log.G(ctx).WithFields(logrus.Fields{
internal/uvm/create_lcow.go:176:		log.G(ctx).WithFields(logrus.Fields{
internal/uvm/create_lcow.go:190:			log.G(ctx).WithFields(logrus.Fields{
internal/uvm/create_lcow.go:542:		log.G(ctx).WithField("options", log.Format(ctx, opts)).Trace("makeLCOWDoc")
internal/uvm/create_lcow.go:630:		log.G(ctx).WithField("resource-partition-id", opts.ResourcePartitionID.String()).Debug("setting resource partition ID")
internal/uvm/create_lcow.go:897:		log.G(ctx).WithField("options", log.Format(ctx, opts)).Debug("uvm::CreateLCOW options")
internal/uvm/create_lcow.go:943:			log.G(ctx).WithFields(logrus.Fields{
internal/uvm/create_lcow.go:951:			log.G(ctx).WithFields(logrus.Fields{
internal/uvm/create_lcow.go:965:		log.G(ctx).WithField("uvm", log.Format(ctx, uvm)).Trace("create_lcow::CreateLCOW uvm.create result")
internal/uvm/create_lcow.go:986:		log.G(ctx).WithField("vmID", uvm.runtimeID).Debug("Using external GCS bridge")
internal/uvm/create_wcow.go:127:	log.G(ctx).WithFields(logrus.Fields{
internal/uvm/create_wcow.go:292:		log.G(ctx).WithField("resource-partition-id", opts.ResourcePartitionID.String()).Debug("setting resource partition ID")
internal/uvm/create_wcow.go:564:	log.G(ctx).WithField("options", log.Format(ctx, opts)).Debug("uvm::CreateWCOW options")
internal/uvm/create_wcow.go:600:			log.G(ctx).WithFields(logrus.Fields{
internal/uvm/create_wcow.go:608:			log.G(ctx).WithFields(logrus.Fields{
internal/uvm/log_wcow.go:33:		log.G(ctx).WithField("os", uvm.operatingSystem).Error("Log forwarding not supported for this OS")
internal/uvm/network.go:100:	l := log.G(ctx).WithField("netns-id", netNS)
internal/uvm/network.go:313:			log.G(ctx).WithFields(logrus.Fields{
internal/uvm/security_policy.go:60:				log.G(ctx).WithField("filePath", fullFilePath).Debug("UVM reference info file not found")
internal/uvm/start.go:78:	e := log.G(ctx).WithField(logfields.UVMID, uvm.id)
internal/uvm/stats.go:60:		log.G(ctx).WithFields(logrus.Fields{
internal/uvm/stats.go:78:	log.G(ctx).WithField("vmID", vmIDStr).Debug("looking up vmmem")
internal/uvm/stats.go:91:			log.G(ctx).WithField("pid", pid).Debug("failed to check process")
internal/uvm/stats.go:95:			log.G(ctx).WithField("pid", pid).Debug("found vmmem match")
internal/uvm/vpmem.go:64:			log.G(ctx).WithFields(logrus.Fields{
internal/uvm/vpmem.go:80:			log.G(ctx).WithFields(logrus.Fields{
internal/uvm/vpmem.go:173:	log.G(ctx).WithFields(logrus.Fields{
internal/uvm/vpmem_mapped.go:123:	log.G(ctx).WithFields(logrus.Fields{
internal/uvm/vpmem_mapped.go:138:	log.G(ctx).WithFields(logrus.Fields{
internal/uvm/vpmem_mapped.go:164:	log.G(ctx).WithFields(logrus.Fields{
internal/uvm/vpmem_mapped.go:177:				log.G(ctx).WithFields(logrus.Fields{
internal/uvm/vpmem_mapped.go:213:		log.G(ctx).WithFields(logrus.Fields{
internal/uvm/vsmb.go:186:		log.G(ctx).WithField("path", hostPath).Info("Forcing NoDirectmap for VSMB mount")
internal/uvm/vsmb.go:216:		log.G(ctx).WithFields(logrus.Fields{
internal/uvm/vsmb.go:301:		log.G(ctx).WithFields(logrus.Fields{
internal/uvm/wait.go:22:	logrus.WithField(logfields.UVMID, uvm.id).Debug("uvm exited, waiting for output processing to complete")
internal/uvmfolder/locate.go:35:	log.G(ctx).WithFields(logrus.Fields{
internal/verity/verity.go:41:	log.G(ctx).WithFields(logrus.Fields{
internal/vhdx/info.go:169:			log.G(ctx).WithFields(logrus.Fields{
internal/vhdx/info.go:183:			log.G(ctx).WithFields(logrus.Fields{
internal/vhdx/info.go:226:	log.G(ctx).WithFields(logrus.Fields{
internal/vm/guestmanager/guest.go:40:		log: log.G(ctx).WithField(logfields.UVMID, uvm.ID()),
internal/vm/vmmanager/uvm.go:59:	log.G(ctx).WithFields(logrus.Fields{
internal/vm/vmutils/gcs_logs.go:68:					logrus.WithFields(logrus.Fields{
internal/vm/vmutils/gcs_logs.go:79:					logrus.WithFields(logrus.Fields{
internal/vm/vmutils/normalize.go:28:		log.G(ctx).WithFields(logrus.Fields{
internal/vm/vmutils/normalize.go:49:		log.G(ctx).WithFields(logrus.Fields{
internal/vm/vmutils/numa.go:65:			log.G(ctx).WithFields(logrus.Fields{
internal/vm/vmutils/numa.go:72:			log.G(ctx).WithFields(logrus.Fields{
internal/vm/vmutils/numa.go:107:			entry.WithFields(logrus.Fields{
internal/vm/vmutils/numa.go:139:		entry.WithField("numa", log.Format(ctx, numa)).Debug("created explicit NUMA topology")
internal/vm/vmutils/utils.go:30:			log.G(ctx).WithField("filePath", fullFilePath).Debug("UVM reference info file not found")
internal/vm/vmutils/utils.go:59:		entry.WithField("options", log.Format(ctx, shimOpts)).Debug("parsed runtime options")
internal/vm/vmutils/vmmem.go:62:	log.G(ctx).WithField("vmID", vmIDStr).Debug("looking up vmmem via LookupAccount")
internal/vm/vmutils/vmmem.go:101:			log.G(ctx).WithField("pid", pe32.ProcessID).Debug("found vmmem match")
internal/vmcompute/vmcompute.go:95:		log.G(ctx).WithFields(logrus.Fields{
internal/vmcompute/vmcompute.go:108:			log.G(ctx).WithField(logfields.Timeout, trueTimeout).
internal/wclayer/cim/mount.go:69:	log.G(ctx).WithFields(logrus.Fields{
internal/wclayer/cim/mount.go:128:	log.L.WithFields(logrus.Fields{
internal/winapi/cimfs/cimfs.go:54:		logrus.WithField("path", windows.UTF16ToString(buf[:n])).Info("loaded cimfs.dll")
internal/winapi/cimwriter/cimwriter.go:56:		logrus.WithField("path", windows.UTF16ToString(buf[:n])).Info("loaded cimwriter.dll")
internal/windevice/devicequery.go:238:	log.G(ctx).WithFields(logrus.Fields{
internal/windevice/devicequery.go:253:	log.G(ctx).WithField("interface list size", interfaceListSize).Trace("retrieved device interface list size")
internal/windevice/devicequery.go:293:		log.G(ctx).WithFields(logrus.Fields{
pkg/cimfs/cim_writer_windows.go:378:	log.G(ctx).WithFields(logrus.Fields{
pkg/ociwclayer/cim/import.go:38:	log.G(ctx).WithFields(logrus.Fields{
pkg/ociwclayer/cim/import.go:129:	log.G(ctx).WithField("layer", layer).Debug("Importing block CIM layer from tar")
pkg/ociwclayer/cim/import.go:143:	log.G(ctx).WithField("config", *config).Debug("layer import config")
pkg/ociwclayer/cim/import.go:303:	log.G(ctx).WithFields(logrus.Fields{
pkg/securitypolicy/securitypolicy_options.go:113:	log.G(ctx).WithField("fragment", fmt.Sprintf("%+v", fragment)).Debug("VerifyAndExtractFragment")
pkg/securitypolicy/securitypolicy_options.go:139:	log.G(ctx).WithFields(logrus.Fields{
pkg/securitypolicy/securitypolicy_options.go:146:	log.G(ctx).WithFields(logrus.Fields{
pkg/securitypolicy/securitypolicyenforcer_rego.go:326:	log.G(ctx).WithField("policyDecision", string(decisionJSON))
test/functional/main_test.go:182:		log.G(ctx).WithField("features", flagFeatures.String()).Debug("provided features")
test/functional/main_test.go:201:			logrus.WithField("image", wcow.nanoserver.Image).Info("using Nano Server image")
test/functional/main_test.go:202:			logrus.WithField("image", wcow.servercore.Image).Info("using Server Core image")
test/functional/main_test.go:220:					log.G(ctx).WithFields(logrus.Fields{
test/functional/main_test.go:368:	e := log.G(ctx).WithFields(logrus.Fields{
test/functional/main_test.go:377:		e.WithFields(logrus.Fields{
test/functional/main_test.go:382:		e.WithField(
test/internal/layers/lazy.go:62:	log.G(ctx).WithFields(logrus.Fields{
test/internal/layers/lazy.go:124:	log.G(ctx).WithFields(logrus.Fields{
test/internal/layers/lazy.go:248:	log.L.WithField("privileges", privs).Infof("enableing process privileges")
test/internal/layers/lazy.go:288:		log.L.WithFields(logrus.Fields{
test/internal/layers/lazy.go:294:		log.L.WithField("path", dir).Info("added MS Defender exclusion for image layers directory")
vendor/github.com/Microsoft/cosesign1go/pkg/cosesign1/create.go:57:		logrus.WithField("key", signingKey).Debug("parsed EC signing (private) key")
vendor/github.com/Microsoft/cosesign1go/pkg/cosesign1/create.go:63:			logrus.WithField("key", signingKey).Debug("parsed PKCS8 signing (private) key")
vendor/github.com/Microsoft/cosesign1go/pkg/cosesign1/create.go:70:			logrus.WithField("key", signingKey).Debug("parsed PKCS1 signing (private) key")
vendor/github.com/Microsoft/cosesign1go/pkg/cosesign1/create.go:88:		logrus.WithField("leaf cert", fmt.Sprintf("%v", *chainCerts[0])).Debug("parsed cert chain for leaf")
vendor/github.com/Microsoft/go-winio/pkg/etw/fieldopt.go:19:// WithFields returns the variadic arguments as a single slice.
vendor/github.com/Microsoft/go-winio/pkg/etw/fieldopt.go:20:func WithFields(opts ...FieldOpt) []FieldOpt {
vendor/github.com/containerd/containerd/v2/core/mount/temp.go:50:			log.G(ctx).WithError(uerr).WithField("dir", root).Error("failed to remove mount temp dir")
vendor/github.com/containerd/containerd/v2/pkg/shim/shim.go:196:	ctx = log.WithLogger(ctx, log.G(ctx).WithField("runtime", manager.Name()))
vendor/github.com/containerd/containerd/v2/pkg/shim/shim.go:259:		logger := log.G(ctx).WithFields(log.Fields{
vendor/github.com/containerd/containerd/v2/pkg/shim/shim.go:350:		log.G(ctx).WithFields(log.Fields{"id": pID, "type": p.Type}).Debug("loading plugin")
vendor/github.com/containerd/containerd/v2/pkg/shim/shim.go:384:				log.G(ctx).WithFields(log.Fields{"id": pID, "type": p.Type, "error": err}).Info("skip loading plugin")
vendor/github.com/containerd/containerd/v2/pkg/shim/shim.go:391:			log.G(ctx).WithField("id", pID).Debug("registering ttrpc service")
vendor/github.com/containerd/containerd/v2/pkg/shim/shim.go:469:	logger := log.G(ctx).WithFields(log.Fields{
vendor/github.com/containerd/containerd/v2/pkg/shim/shim_unix.go:69:	log.L.WithField("socket", path).Debug("serving api on socket")
vendor/github.com/containerd/containerd/v2/pkg/shim/shim_unix.go:101:			logger.WithField("signal", s).Debugf("Caught exit signal")
vendor/github.com/containerd/log/context.go:62:// Fields type to pass to "WithFields".
vendor/github.com/containerd/log/context.go:66:// [Entry.WithFields]. It's finally logged when Trace, Debug, Info, Warn,
vendor/github.com/containerd/log/context.go:170:// combination with logger.WithField(s) for great effect.
vendor/github.com/containerd/ttrpc/client.go:371:				log.G(c.ctx).WithField("stream", sid).Error("ttrpc: received message on inactive stream")
vendor/github.com/containerd/ttrpc/client.go:379:					log.G(c.ctx).WithFields(log.Fields{"error": err, "stream": sid}).Error("ttrpc: failed to handle message")
vendor/github.com/docker/cli/cli/config/configfile/file.go:183:				logrus.WithError(err).WithField("file", temp.Name()).Debug("Error cleaning up temp file")
vendor/github.com/open-policy-agent/opa/internal/providers/aws/util.go:21:	logger.WithFields(map[string]interface{}{
vendor/github.com/open-policy-agent/opa/logging/logging.go:32:	WithFields(map[string]interface{}) Logger
vendor/github.com/open-policy-agent/opa/logging/logging.go:70:// WithFields provides additional fields to include in log output
vendor/github.com/open-policy-agent/opa/logging/logging.go:71:func (l *StandardLogger) WithFields(fields map[string]interface{}) Logger {
vendor/github.com/open-policy-agent/opa/logging/logging.go:131:		l.logger.WithFields(l.getFields()).Debug(fmt)
vendor/github.com/open-policy-agent/opa/logging/logging.go:134:	l.logger.WithFields(l.getFields()).Debugf(fmt, a...)
vendor/github.com/open-policy-agent/opa/logging/logging.go:140:		l.logger.WithFields(l.getFields()).Info(fmt)
vendor/github.com/open-policy-agent/opa/logging/logging.go:143:	l.logger.WithFields(l.getFields()).Infof(fmt, a...)
vendor/github.com/open-policy-agent/opa/logging/logging.go:149:		l.logger.WithFields(l.getFields()).Error(fmt)
vendor/github.com/open-policy-agent/opa/logging/logging.go:152:	l.logger.WithFields(l.getFields()).Errorf(fmt, a...)
vendor/github.com/open-policy-agent/opa/logging/logging.go:158:		l.logger.WithFields(l.getFields()).Warn(fmt)
vendor/github.com/open-policy-agent/opa/logging/logging.go:161:	l.logger.WithFields(l.getFields()).Warnf(fmt, a...)
vendor/github.com/open-policy-agent/opa/logging/logging.go:177:// WithFields provides additional fields to include in log output.
vendor/github.com/open-policy-agent/opa/logging/logging.go:179:func (l *NoOpLogger) WithFields(fields map[string]interface{}) Logger {
vendor/github.com/open-policy-agent/opa/plugins/plugins.go:1081:					m.logger.WithFields(map[string]interface{}{"err": err}).Debug("Unable to send OPA telemetry report.")
vendor/github.com/open-policy-agent/opa/plugins/rest/rest.go:333:		c.logger.WithFields(c.loggerFields).Debug("Sending request.")
vendor/github.com/open-policy-agent/opa/plugins/rest/rest.go:356:		c.logger.WithFields(c.loggerFields).Debug("Received response.")
vendor/github.com/sirupsen/logrus/CHANGELOG.md:199:* performance: avoid re-allocations on `WithFields` (#335)
vendor/github.com/sirupsen/logrus/README.md:126:  log.WithFields(log.Fields{
vendor/github.com/sirupsen/logrus/README.md:158:  log.WithFields(log.Fields{
vendor/github.com/sirupsen/logrus/README.md:163:  log.WithFields(log.Fields{
vendor/github.com/sirupsen/logrus/README.md:168:  log.WithFields(log.Fields{
vendor/github.com/sirupsen/logrus/README.md:174:  // the logrus.Entry returned from WithFields()
vendor/github.com/sirupsen/logrus/README.md:175:  contextLogger := log.WithFields(log.Fields{
vendor/github.com/sirupsen/logrus/README.md:212:  log.WithFields(logrus.Fields{
vendor/github.com/sirupsen/logrus/README.md:227:log.WithFields(log.Fields{
vendor/github.com/sirupsen/logrus/README.md:237:hours. The `WithFields` call is optional.
vendor/github.com/sirupsen/logrus/README.md:248:`log.WithFields(log.Fields{"request_id": request_id, "user_ip": user_ip})` on
vendor/github.com/sirupsen/logrus/README.md:252:requestLogger := log.WithFields(log.Fields{"request_id": request_id, "user_ip": user_ip})
vendor/github.com/sirupsen/logrus/README.md:324:Besides the fields added with `WithField` or `WithFields` some fields are
vendor/github.com/sirupsen/logrus/doc.go:14:    log.WithFields(log.Fields{
vendor/github.com/sirupsen/logrus/entry.go:41:// the fields passed with WithField{,s}. It's finally logged when Trace, Debug,
vendor/github.com/sirupsen/logrus/entry.go:107:	return entry.WithField(ErrorKey, err)
vendor/github.com/sirupsen/logrus/entry.go:120:func (entry *Entry) WithField(key string, value interface{}) *Entry {
vendor/github.com/sirupsen/logrus/entry.go:121:	return entry.WithFields(Fields{key: value})
vendor/github.com/sirupsen/logrus/entry.go:125:func (entry *Entry) WithFields(fields Fields) *Entry {
vendor/github.com/sirupsen/logrus/exported.go:56:	return std.WithField(ErrorKey, err)
vendor/github.com/sirupsen/logrus/exported.go:64:// WithField creates an entry from the standard logger and adds a field to
vendor/github.com/sirupsen/logrus/exported.go:65:// it. If you want multiple fields, use `WithFields`.
vendor/github.com/sirupsen/logrus/exported.go:69:func WithField(key string, value interface{}) *Entry {
vendor/github.com/sirupsen/logrus/exported.go:70:	return std.WithField(key, value)
vendor/github.com/sirupsen/logrus/exported.go:73:// WithFields creates an entry from the standard logger and adds multiple
vendor/github.com/sirupsen/logrus/exported.go:74:// fields to it. This is simply a helper for `WithField`, invoking it
vendor/github.com/sirupsen/logrus/exported.go:79:func WithFields(fields Fields) *Entry {
vendor/github.com/sirupsen/logrus/exported.go:80:	return std.WithFields(fields)
vendor/github.com/sirupsen/logrus/formatter.go:23:// Any additional fields added with `WithField` or `WithFields` are also in
vendor/github.com/sirupsen/logrus/formatter.go:33://  logrus.WithField("level", 1).Info("hello")
vendor/github.com/sirupsen/logrus/logger.go:111:// WithField allocates a new entry and adds a field to it.
vendor/github.com/sirupsen/logrus/logger.go:114:// If you want multiple fields, use `WithFields`.
vendor/github.com/sirupsen/logrus/logger.go:115:func (logger *Logger) WithField(key string, value interface{}) *Entry {
vendor/github.com/sirupsen/logrus/logger.go:118:	return entry.WithField(key, value)
vendor/github.com/sirupsen/logrus/logger.go:121:// Adds a struct of fields to the log entry. All it does is call `WithField` for
vendor/github.com/sirupsen/logrus/logger.go:123:func (logger *Logger) WithFields(fields Fields) *Entry {
vendor/github.com/sirupsen/logrus/logger.go:126:	return entry.WithFields(fields)
vendor/github.com/sirupsen/logrus/logrus.go:9:// Fields type, used to pass to `WithFields`.
vendor/github.com/sirupsen/logrus/logrus.go:140:	WithField(key string, value interface{}) *Entry
vendor/github.com/sirupsen/logrus/logrus.go:141:	WithFields(fields Fields) *Entry
```

## WithFields usages

```txt
cmd/containerd-shim-lcow-v2/service/plugin/plugin.go:50:		etw.WithFields(
cmd/containerd-shim-runhcs-v1/exec_hcs.go:46:	log.G(ctx).WithFields(logrus.Fields{
cmd/containerd-shim-runhcs-v1/exec_hcs.go:205:		Log: log.G(ctx).WithFields(logrus.Fields{
cmd/containerd-shim-runhcs-v1/exec_wcow_podsandbox.go:22:	log.G(ctx).WithFields(logrus.Fields{
cmd/containerd-shim-runhcs-v1/main.go:98:		etw.WithFields(
cmd/containerd-shim-runhcs-v1/task_hcs.go:191:	log.G(ctx).WithFields(logrus.Fields{
cmd/containerd-shim-runhcs-v1/task_hcs.go:436:				log.G(ctx).WithFields(logrus.Fields{
cmd/gcs-sidecar/main.go:218:	logrus.WithFields(logrus.Fields{
cmd/gcs/main.go:77:		entry := logrus.WithFields(logrus.Fields{
cmd/gcs/main.go:94:			entry.WithFields(memoryLogFormat(metrics)).Warn(msg)
cmd/gcs/main.go:111:			logrus.WithFields(logrus.Fields{
cmd/gcs/main.go:231:			logrus.WithFields(logrus.Fields{
cmd/gcs/main.go:252:		logrus.WithFields(logrus.Fields{
cmd/gcs/main.go:265:		logrus.WithFields(logrus.Fields{
cmd/gcs/main.go:281:	logrus.WithFields(logrus.Fields{
cmd/gcs/main.go:380:			logrus.WithFields(logrus.Fields{
cmd/gcs/main.go:424:		logrus.WithFields(logrus.Fields{
cmd/ncproxy/run.go:277:	log.G(ctx).WithFields(logrus.Fields{
cmd/ncproxy/server.go:122:		log.G(ctx).WithFields(logrus.Fields{
cmd/ncproxy/server.go:131:		log.G(ctx).WithFields(logrus.Fields{
cmd/runhcs/container.go:493:			logrus.WithFields(logrus.Fields{
cmd/runhcs/container.go:524:	logrus.WithFields(logrus.Fields{
cmd/runhcs/vm.go:152:	logrus.WithFields(logrus.Fields{
hcn/hcnnamespace.go:392:		logrus.WithFields(f).
hcn/hcnsupport.go:124:	log.L.WithFields(logrus.Fields{
internal/bridgeutils/commonutils/utilities.go:59:			logrus.WithFields(logrus.Fields{
internal/builder/vm/lcow/boot.go:38:		log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/boot.go:92:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/boot.go:165:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/confidential.go:141:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/confidential.go:177:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/confidential.go:189:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/confidential.go:228:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/devices.go:38:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/devices.go:61:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/devices.go:96:			log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/devices.go:131:		log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/devices.go:187:				log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/devices.go:204:		log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/devices.go:215:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/kernel_args.go:100:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/kernel_args.go:161:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/specs.go:338:		log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/specs.go:359:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/topology.go:74:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/topology.go:116:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/topology.go:158:	log.G(ctx).WithFields(logrus.Fields{
internal/cmd/io.go:71:		log = log.WithFields(logrus.Fields{
internal/cmd/io_binary.go:110:	log.G(ctx).WithFields(logrus.Fields{
internal/cmd/io_npipe.go:26:	log.G(ctx).WithFields(logrus.Fields{
internal/cmd/io_npipe.go:111:				log.G(nprw.ctx).WithFields(logrus.Fields{
internal/computecore/computecore.go:137:		log.G(ctx).WithFields(logrus.Fields{
internal/devices/pnp.go:60:		log.G(ctx).WithFields(logrus.Fields{
internal/gcs-sidecar/bridge.go:141:		logrus.WithFields(logrus.Fields{
internal/gcs-sidecar/handlers.go:729:							log.G(ctx).WithFields(map[string]interface{}{
internal/gcs-sidecar/host.go:108:	logrus.WithFields(logrus.Fields{
internal/gcs/bridge.go:296:		brdg.log.WithFields(logrus.Fields{
internal/gcs/bridge.go:316:					brdg.log.WithFields(logrus.Fields{
internal/gcs/bridge.go:406:		brdg.log.WithFields(logrus.Fields{
internal/gcs/process.go:257:			log.G(ctx).WithFields(logrus.Fields{
internal/guest/bridge/bridge.go:83:		logrus.WithFields(logrus.Fields{
internal/guest/kmsg/kmsg.go:135:			logrus.WithFields(logrus.Fields{
internal/guest/kmsg/kmsg.go:141:				logrus.WithFields(entry.logFormat()).Info("kmsg read")
internal/guest/network/netns.go:197:		log.G(ctx).WithFields(logrus.Fields{
internal/guest/network/netns.go:210:		log.G(ctx).WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/container.go:102:				entity.WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/container.go:183:	logrus.WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/container.go:217:	log.G(ctx).WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/process.go:113:				log.G(ctx).WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/process.go:244:		log.G(ctx).WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/uvm.go:161:		logrus.WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/uvm.go:335:		logrus.WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/uvm.go:365:		logrus.WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/uvm.go:569:		log.G(ctx).WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/uvm.go:1461:		logrus.WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/uvm.go:1486:	logrus.WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/uvm.go:1525:	logrus.WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/uvm.go:1563:	logrus.WithFields(logrus.Fields{
internal/guest/runtime/runc/container.go:84:	logrus.WithFields(logrus.Fields{
internal/guest/runtime/runc/container.go:248:	logrus.WithFields(logrus.Fields{
internal/guest/spec/spec.go:52:	logrus.WithFields(logrus.Fields{
internal/guest/spec/spec.go:475:			log.G(ctx).WithFields(logrus.Fields{
internal/guest/spec/spec_devices.go:71:				entry.WithFields(logrus.Fields{
internal/guest/stdio/stdio.go:171:		logrus.WithFields(logrus.Fields{
internal/guest/stdio/stdio.go:187:			logrus.WithFields(logrus.Fields{
internal/guest/stdio/stdio.go:193:		logrus.WithFields(logrus.Fields{
internal/guest/stdio/stdio.go:199:		logrus.WithFields(logrus.Fields{
internal/guest/stdio/stdio.go:213:				logrus.WithFields(logrus.Fields{
internal/guest/stdio/stdio.go:219:				logrus.WithFields(logrus.Fields{
internal/guest/stdio/stdio.go:289:					logrus.WithFields(logrus.Fields{
internal/guest/stdio/stdio.go:337:				logrus.WithFields(logrus.Fields{
internal/guest/stdio/stdio.go:348:				logrus.WithFields(logrus.Fields{
internal/guest/storage/crypt/crypt.go:51:			log.G(ctx).WithError(err).WithFields(logrus.Fields{
internal/guest/storage/overlay/overlay.go:49:	log.G(ctx).WithFields(logrus.Fields{
internal/guest/storage/scsi/scsi.go:176:		log.G(ctx).WithFields(logrus.Fields{
internal/guest/transport/devnull.go:23:	logrus.WithFields(logrus.Fields{
internal/guest/transport/vsock.go:26:	logrus.WithFields(logrus.Fields{
internal/hcs/callback.go:149:	log := logrus.WithFields(logrus.Fields{
internal/hcs/system.go:553:	logEntry.WithFields(logrus.Fields{
internal/hcsoci/create.go:131:	log.G(ctx).WithFields(logrus.Fields{
internal/hcsoci/hcsdoc_wcow.go:222:		log.G(ctx).WithFields(logrus.Fields{
internal/hcsoci/hcsdoc_wcow.go:246:			log.G(ctx).WithFields(logrus.Fields{
internal/hcsoci/network.go:29:	log.G(ctx).WithFields(logrus.Fields{
internal/hcsoci/network.go:43:		log.G(ctx).WithFields(logrus.Fields{
internal/hcsoci/resources.go:25:		log.G(ctx).WithFields(logrus.Fields{
internal/hcsoci/resources.go:40:		log.G(ctx).WithFields(logrus.Fields{
internal/jobcontainers/mounts.go:104:			log.G(ctx).WithFields(logrus.Fields{
internal/jobobject/iocp.go:57:				log.G(ctx).WithFields(logrus.Fields{
internal/jobobject/iocp.go:69:				log.G(ctx).WithFields(logrus.Fields{
internal/layers/lcow.go:64:			log.G(ctx).WithFields(logrus.Fields{
internal/layers/lcow.go:198:			log.G(ctx).WithFields(logrus.Fields{
internal/layers/lcow.go:222:	log.G(ctx).WithFields(logrus.Fields{
internal/layers/wcow_mount.go:252:	log.G(ctx).WithFields(logrus.Fields{
internal/layers/wcow_mount.go:341:	log.G(ctx).WithFields(logrus.Fields{
internal/layers/wcow_mount.go:392:			log.G(ctx).WithFields(logrus.Fields{
internal/layers/wcow_mount.go:512:	log.G(ctx).WithFields(logrus.Fields{
internal/layers/wcow_mount.go:539:	log.G(ctx).WithFields(logrus.Fields{
internal/layers/wcow_mount.go:582:	log.G(ctx).WithFields(logrus.Fields{
internal/layers/wcow_mount.go:612:	log.G(ctx).WithFields(logrus.Fields{
internal/lcow/common.go:48:	log.G(ctx).WithFields(logrus.Fields{
internal/lcow/disk.go:29:	log.G(ctx).WithFields(logrus.Fields{
internal/lcow/disk.go:43:	log.G(ctx).WithFields(logrus.Fields{
internal/lcow/scratch.go:47:	log.G(ctx).WithFields(logrus.Fields{
internal/lcow/scratch.go:59:			log.G(ctx).WithFields(logrus.Fields{
internal/lcow/scratch.go:93:	log.G(ctx).WithFields(logrus.Fields{
internal/log/context.go:52://	entry := GetEntry(ctx).WithFields(fields)
internal/log/context.go:59:		e = e.WithFields(fields)
internal/log/format.go:69:		G(ctx).WithFields(logrus.Fields{
internal/oc/exporter.go:34:		logrus.WithFields(logrus.Fields{
internal/oc/exporter.go:47:	// can skip overhead in entry.WithFields() and add them directly to entry.Data.
internal/oci/annotations.go:46:					log.G(ctx).WithFields(logrus.Fields{
internal/oci/annotations.go:69:		log.G(ctx).WithFields(logrus.Fields{
internal/oci/annotations.go:118:		entry := log.G(ctx).WithFields(logrus.Fields{
internal/oci/annotations.go:230:		entry := log.G(ctx).WithFields(logrus.Fields{
internal/oci/annotations.go:250:			entry.WithFields(logrus.Fields{
internal/oci/annotations.go:405:	entry := log.G(ctx).WithFields(logrus.Fields{
internal/oci/uvm.go:140:			log.G(ctx).WithFields(logrus.Fields{
internal/oci/uvm.go:294:			log.G(ctx).WithFields(logrus.Fields{
internal/shim/shim.go:261:		logger := log.G(ctx).WithFields(log.Fields{
internal/shim/shim.go:354:		log.G(ctx).WithFields(log.Fields{"id": pID, "type": p.Type}).Debug("loading plugin")
internal/shim/shim.go:388:				log.G(ctx).WithFields(log.Fields{"id": pID, "type": p.Type, "error": err}).Info("skip loading plugin")
internal/shim/shim.go:485:	logger := log.G(ctx).WithFields(log.Fields{
internal/tools/networkagent/main.go:470:	log.G(ctx).WithFields(logrus.Fields{
internal/tools/rootfs/main.go:70:				logrus.WithFields(logrus.Fields{
internal/tools/rootfs/merge.go:141:	logrus.WithFields(logrus.Fields{
internal/tools/rootfs/merge.go:178:	logrus.WithFields(logrus.Fields{
internal/tools/rootfs/merge.go:204:		logrus.WithFields(logrus.Fields{
internal/tools/rootfs/merge.go:259:		entry := logrus.WithFields(logrus.Fields{
internal/tools/rootfs/merge.go:321:			entry.WithFields(logrus.Fields{
internal/tools/rootfs/merge.go:336:			entry.WithFields(logrus.Fields{
internal/tools/uvmboot/lcow.go:300:			entry = entry.WithFields(logrus.Fields{
internal/tools/uvmboot/mounts.go:34:		log.G(ctx).WithFields(logrus.Fields{
internal/tools/uvmboot/mounts.go:62:		log.G(ctx).WithFields(logrus.Fields{
internal/uvm/cimfs.go:121:					log.G(ctx).WithFields(logrus.Fields{
internal/uvm/cimfs.go:140:		log.G(ctx).WithFields(logrus.Fields{
internal/uvm/computeagent.go:66:	log.G(ctx).WithFields(logrus.Fields{
internal/uvm/computeagent.go:84:	log.G(ctx).WithFields(logrus.Fields{
internal/uvm/computeagent.go:100:	log.G(ctx).WithFields(logrus.Fields{
internal/uvm/computeagent.go:166:	log.G(ctx).WithFields(logrus.Fields{
internal/uvm/computeagent.go:216:	log.G(ctx).WithFields(logrus.Fields{
internal/uvm/create.go:282:	log.G(ctx).WithFields(logrus.Fields{
internal/uvm/create_lcow.go:159:		log.G(ctx).WithFields(logrus.Fields{
internal/uvm/create_lcow.go:176:		log.G(ctx).WithFields(logrus.Fields{
internal/uvm/create_lcow.go:190:			log.G(ctx).WithFields(logrus.Fields{
internal/uvm/create_lcow.go:943:			log.G(ctx).WithFields(logrus.Fields{
internal/uvm/create_lcow.go:951:			log.G(ctx).WithFields(logrus.Fields{
internal/uvm/create_wcow.go:127:	log.G(ctx).WithFields(logrus.Fields{
internal/uvm/create_wcow.go:600:			log.G(ctx).WithFields(logrus.Fields{
internal/uvm/create_wcow.go:608:			log.G(ctx).WithFields(logrus.Fields{
internal/uvm/network.go:313:			log.G(ctx).WithFields(logrus.Fields{
internal/uvm/stats.go:60:		log.G(ctx).WithFields(logrus.Fields{
internal/uvm/vpmem.go:64:			log.G(ctx).WithFields(logrus.Fields{
internal/uvm/vpmem.go:80:			log.G(ctx).WithFields(logrus.Fields{
internal/uvm/vpmem.go:173:	log.G(ctx).WithFields(logrus.Fields{
internal/uvm/vpmem_mapped.go:123:	log.G(ctx).WithFields(logrus.Fields{
internal/uvm/vpmem_mapped.go:138:	log.G(ctx).WithFields(logrus.Fields{
internal/uvm/vpmem_mapped.go:164:	log.G(ctx).WithFields(logrus.Fields{
internal/uvm/vpmem_mapped.go:177:				log.G(ctx).WithFields(logrus.Fields{
internal/uvm/vpmem_mapped.go:213:		log.G(ctx).WithFields(logrus.Fields{
internal/uvm/vsmb.go:216:		log.G(ctx).WithFields(logrus.Fields{
internal/uvm/vsmb.go:301:		log.G(ctx).WithFields(logrus.Fields{
internal/uvmfolder/locate.go:35:	log.G(ctx).WithFields(logrus.Fields{
internal/verity/verity.go:41:	log.G(ctx).WithFields(logrus.Fields{
internal/vhdx/info.go:169:			log.G(ctx).WithFields(logrus.Fields{
internal/vhdx/info.go:183:			log.G(ctx).WithFields(logrus.Fields{
internal/vhdx/info.go:226:	log.G(ctx).WithFields(logrus.Fields{
internal/vm/vmmanager/uvm.go:59:	log.G(ctx).WithFields(logrus.Fields{
internal/vm/vmutils/gcs_logs.go:68:					logrus.WithFields(logrus.Fields{
internal/vm/vmutils/gcs_logs.go:79:					logrus.WithFields(logrus.Fields{
internal/vm/vmutils/normalize.go:28:		log.G(ctx).WithFields(logrus.Fields{
internal/vm/vmutils/normalize.go:49:		log.G(ctx).WithFields(logrus.Fields{
internal/vm/vmutils/numa.go:65:			log.G(ctx).WithFields(logrus.Fields{
internal/vm/vmutils/numa.go:72:			log.G(ctx).WithFields(logrus.Fields{
internal/vm/vmutils/numa.go:107:			entry.WithFields(logrus.Fields{
internal/vmcompute/vmcompute.go:95:		log.G(ctx).WithFields(logrus.Fields{
internal/wclayer/cim/mount.go:69:	log.G(ctx).WithFields(logrus.Fields{
internal/wclayer/cim/mount.go:128:	log.L.WithFields(logrus.Fields{
internal/windevice/devicequery.go:238:	log.G(ctx).WithFields(logrus.Fields{
internal/windevice/devicequery.go:293:		log.G(ctx).WithFields(logrus.Fields{
pkg/cimfs/cim_writer_windows.go:378:	log.G(ctx).WithFields(logrus.Fields{
pkg/ociwclayer/cim/import.go:38:	log.G(ctx).WithFields(logrus.Fields{
pkg/ociwclayer/cim/import.go:303:	log.G(ctx).WithFields(logrus.Fields{
pkg/securitypolicy/securitypolicy_options.go:139:	log.G(ctx).WithFields(logrus.Fields{
pkg/securitypolicy/securitypolicy_options.go:146:	log.G(ctx).WithFields(logrus.Fields{
test/functional/main_test.go:220:					log.G(ctx).WithFields(logrus.Fields{
test/functional/main_test.go:368:	e := log.G(ctx).WithFields(logrus.Fields{
test/functional/main_test.go:377:		e.WithFields(logrus.Fields{
test/internal/layers/lazy.go:62:	log.G(ctx).WithFields(logrus.Fields{
test/internal/layers/lazy.go:124:	log.G(ctx).WithFields(logrus.Fields{
test/internal/layers/lazy.go:288:		log.L.WithFields(logrus.Fields{
vendor/github.com/Microsoft/go-winio/pkg/etw/fieldopt.go:19:// WithFields returns the variadic arguments as a single slice.
vendor/github.com/Microsoft/go-winio/pkg/etw/fieldopt.go:20:func WithFields(opts ...FieldOpt) []FieldOpt {
vendor/github.com/containerd/containerd/v2/pkg/shim/shim.go:259:		logger := log.G(ctx).WithFields(log.Fields{
vendor/github.com/containerd/containerd/v2/pkg/shim/shim.go:350:		log.G(ctx).WithFields(log.Fields{"id": pID, "type": p.Type}).Debug("loading plugin")
vendor/github.com/containerd/containerd/v2/pkg/shim/shim.go:384:				log.G(ctx).WithFields(log.Fields{"id": pID, "type": p.Type, "error": err}).Info("skip loading plugin")
vendor/github.com/containerd/containerd/v2/pkg/shim/shim.go:469:	logger := log.G(ctx).WithFields(log.Fields{
vendor/github.com/containerd/log/context.go:62:// Fields type to pass to "WithFields".
vendor/github.com/containerd/log/context.go:66:// [Entry.WithFields]. It's finally logged when Trace, Debug, Info, Warn,
vendor/github.com/containerd/ttrpc/client.go:379:					log.G(c.ctx).WithFields(log.Fields{"error": err, "stream": sid}).Error("ttrpc: failed to handle message")
vendor/github.com/open-policy-agent/opa/internal/providers/aws/util.go:21:	logger.WithFields(map[string]interface{}{
vendor/github.com/open-policy-agent/opa/logging/logging.go:32:	WithFields(map[string]interface{}) Logger
vendor/github.com/open-policy-agent/opa/logging/logging.go:70:// WithFields provides additional fields to include in log output
vendor/github.com/open-policy-agent/opa/logging/logging.go:71:func (l *StandardLogger) WithFields(fields map[string]interface{}) Logger {
vendor/github.com/open-policy-agent/opa/logging/logging.go:131:		l.logger.WithFields(l.getFields()).Debug(fmt)
vendor/github.com/open-policy-agent/opa/logging/logging.go:134:	l.logger.WithFields(l.getFields()).Debugf(fmt, a...)
vendor/github.com/open-policy-agent/opa/logging/logging.go:140:		l.logger.WithFields(l.getFields()).Info(fmt)
vendor/github.com/open-policy-agent/opa/logging/logging.go:143:	l.logger.WithFields(l.getFields()).Infof(fmt, a...)
vendor/github.com/open-policy-agent/opa/logging/logging.go:149:		l.logger.WithFields(l.getFields()).Error(fmt)
vendor/github.com/open-policy-agent/opa/logging/logging.go:152:	l.logger.WithFields(l.getFields()).Errorf(fmt, a...)
vendor/github.com/open-policy-agent/opa/logging/logging.go:158:		l.logger.WithFields(l.getFields()).Warn(fmt)
vendor/github.com/open-policy-agent/opa/logging/logging.go:161:	l.logger.WithFields(l.getFields()).Warnf(fmt, a...)
vendor/github.com/open-policy-agent/opa/logging/logging.go:177:// WithFields provides additional fields to include in log output.
vendor/github.com/open-policy-agent/opa/logging/logging.go:179:func (l *NoOpLogger) WithFields(fields map[string]interface{}) Logger {
vendor/github.com/open-policy-agent/opa/plugins/plugins.go:1081:					m.logger.WithFields(map[string]interface{}{"err": err}).Debug("Unable to send OPA telemetry report.")
vendor/github.com/open-policy-agent/opa/plugins/rest/rest.go:333:		c.logger.WithFields(c.loggerFields).Debug("Sending request.")
vendor/github.com/open-policy-agent/opa/plugins/rest/rest.go:356:		c.logger.WithFields(c.loggerFields).Debug("Received response.")
vendor/github.com/sirupsen/logrus/CHANGELOG.md:199:* performance: avoid re-allocations on `WithFields` (#335)
vendor/github.com/sirupsen/logrus/README.md:126:  log.WithFields(log.Fields{
vendor/github.com/sirupsen/logrus/README.md:158:  log.WithFields(log.Fields{
vendor/github.com/sirupsen/logrus/README.md:163:  log.WithFields(log.Fields{
vendor/github.com/sirupsen/logrus/README.md:168:  log.WithFields(log.Fields{
vendor/github.com/sirupsen/logrus/README.md:174:  // the logrus.Entry returned from WithFields()
vendor/github.com/sirupsen/logrus/README.md:175:  contextLogger := log.WithFields(log.Fields{
vendor/github.com/sirupsen/logrus/README.md:212:  log.WithFields(logrus.Fields{
vendor/github.com/sirupsen/logrus/README.md:227:log.WithFields(log.Fields{
vendor/github.com/sirupsen/logrus/README.md:237:hours. The `WithFields` call is optional.
vendor/github.com/sirupsen/logrus/README.md:248:`log.WithFields(log.Fields{"request_id": request_id, "user_ip": user_ip})` on
vendor/github.com/sirupsen/logrus/README.md:252:requestLogger := log.WithFields(log.Fields{"request_id": request_id, "user_ip": user_ip})
vendor/github.com/sirupsen/logrus/README.md:324:Besides the fields added with `WithField` or `WithFields` some fields are
vendor/github.com/sirupsen/logrus/doc.go:14:    log.WithFields(log.Fields{
vendor/github.com/sirupsen/logrus/entry.go:121:	return entry.WithFields(Fields{key: value})
vendor/github.com/sirupsen/logrus/entry.go:125:func (entry *Entry) WithFields(fields Fields) *Entry {
vendor/github.com/sirupsen/logrus/exported.go:65:// it. If you want multiple fields, use `WithFields`.
vendor/github.com/sirupsen/logrus/exported.go:73:// WithFields creates an entry from the standard logger and adds multiple
vendor/github.com/sirupsen/logrus/exported.go:79:func WithFields(fields Fields) *Entry {
vendor/github.com/sirupsen/logrus/exported.go:80:	return std.WithFields(fields)
vendor/github.com/sirupsen/logrus/formatter.go:23:// Any additional fields added with `WithField` or `WithFields` are also in
vendor/github.com/sirupsen/logrus/logger.go:114:// If you want multiple fields, use `WithFields`.
vendor/github.com/sirupsen/logrus/logger.go:123:func (logger *Logger) WithFields(fields Fields) *Entry {
vendor/github.com/sirupsen/logrus/logger.go:126:	return entry.WithFields(fields)
vendor/github.com/sirupsen/logrus/logrus.go:9:// Fields type, used to pass to `WithFields`.
vendor/github.com/sirupsen/logrus/logrus.go:141:	WithFields(fields Fields) *Entry
```

## WithError usages

```txt
cmd/containerd-shim-lcow-v2/manager.go:213:		logrus.WithError(err).Warn("failed to open shim panic log")
cmd/containerd-shim-lcow-v2/service/service.go:116:			log.G(ctx).WithError(err).Error("post event")
cmd/containerd-shim-lcow-v2/service/service_sandbox_internal.go:276:			log.G(ctx).WithError(err).Error("failed to terminate VM during shutdown")
cmd/containerd-shim-runhcs-v1/delete.go:82:			logrus.WithError(err).Warn("failed to open shim panic log")
cmd/containerd-shim-runhcs-v1/exec_hcs.go:460:		log.G(ctx).WithError(err).Error("failed process Wait")
cmd/containerd-shim-runhcs-v1/exec_hcs.go:469:		log.G(ctx).WithError(err).Error("failed to get ExitCode")
cmd/containerd-shim-runhcs-v1/exec_hcs.go:498:			log.G(ctx).WithError(err).Error("failed to publish TaskExitEvent")
cmd/containerd-shim-runhcs-v1/serve.go:219:				logrus.WithError(err).Fatal("containerd-shim: ttrpc server failure")
cmd/containerd-shim-runhcs-v1/task_hcs.go:533:				log.G(ctx).WithError(err).Errorf("failed to delete container state")
cmd/containerd-shim-runhcs-v1/task_hcs.go:634:		log.G(ctx).WithError(err).Error("failed to wait for host virtual machine exit")
cmd/containerd-shim-runhcs-v1/task_hcs.go:675:				log.G(ctx).WithError(err).Error("failed to shutdown container")
cmd/containerd-shim-runhcs-v1/task_hcs.go:683:						log.G(ctx).WithError(err).Error("failed to wait for container shutdown")
cmd/containerd-shim-runhcs-v1/task_hcs.go:687:					log.G(ctx).WithError(err).Error("failed to wait for container shutdown")
cmd/containerd-shim-runhcs-v1/task_hcs.go:694:					log.G(ctx).WithError(err).Error("failed to terminate container")
cmd/containerd-shim-runhcs-v1/task_hcs.go:702:							log.G(ctx).WithError(err).Error("failed to wait for container terminate")
cmd/containerd-shim-runhcs-v1/task_hcs.go:705:						log.G(ctx).WithError(hcs.ErrTimeout).Error("failed to wait for container terminate")
cmd/containerd-shim-runhcs-v1/task_hcs.go:712:				log.G(ctx).WithError(err).Error("failed to release container resources")
cmd/containerd-shim-runhcs-v1/task_hcs.go:717:				log.G(ctx).WithError(err).Error("failed to close container")
cmd/containerd-shim-runhcs-v1/task_hcs.go:737:				log.G(ctx).WithError(err).Error("failed host vm shutdown")
cmd/containerd-shim-runhcs-v1/task_hcs.go:753:			log.G(ctx).WithError(err).Error("failed to publish TaskExitEventTopic")
cmd/containerd-shim-runhcs-v1/task_hcs.go:781:			log.G(ctx).WithError(err).Warn("failed to capture guest stacks")
cmd/containerd-shim-runhcs-v1/task_wcow_podsandbox.go:191:				log.G(ctx).WithError(err).Error("failed to cleanup networking for utility VM")
cmd/containerd-shim-runhcs-v1/task_wcow_podsandbox.go:195:				log.G(ctx).WithError(err).Error("failed host vm shutdown")
cmd/containerd-shim-runhcs-v1/task_wcow_podsandbox.go:211:			log.G(ctx).WithError(err).Error("failed to publish TaskExitEventTopic")
cmd/containerd-shim-runhcs-v1/task_wcow_podsandbox.go:236:		log.G(ctx).WithError(werr).Error("parent wait failed")
cmd/containerd-shim-runhcs-v1/task_wcow_podsandbox.go:268:			log.G(ctx).WithError(err).Warn("failed to capture guest stacks")
cmd/gcs-sidecar/main.go:160:		logrus.WithError(err).Error("error redirecting handle")
cmd/gcs-sidecar/main.go:192:		logrus.WithError(vsmbError).Errorf("VSMB redirector initialization failed.")
cmd/gcs-sidecar/main.go:201:		logrus.WithError(err).Error("error starting listener for sidecar <-> inbox gcs communication")
cmd/gcs-sidecar/main.go:208:		logrus.WithError(err).Error("error accepting inbox GCS connection")
cmd/gcs-sidecar/main.go:223:		logrus.WithError(err).Error("error dialing hcsshim external bridge")
cmd/gcs-sidecar/main.go:231:		logrus.WithError(err).Errorf("failed to start PSP driver")
cmd/gcs-sidecar/main.go:245:		logrus.WithError(err).Error("failed to serve request")
cmd/gcs/main.go:58:			logrus.WithError(err).WithField("cgroup", cgName).Error("failed to read from eventfd")
cmd/gcs/main.go:92:			entry.WithError(err).Error(msg)
cmd/gcs/main.go:296:			logrus.WithError(err).Fatal("failed to set core dump location")
cmd/gcs/main.go:312:		logrus.WithError(err).Fatal("failed to enable hierarchy support for root cgroup")
cmd/gcs/main.go:322:		logrus.WithError(err).Fatal("failed to get sys info")
cmd/gcs/main.go:331:		logrus.WithError(err).Fatal("failed to create containers cgroup")
cmd/gcs/main.go:343:		logrus.WithError(err).Fatal("failed to create containers/virtual-pods cgroup")
cmd/gcs/main.go:349:		logrus.WithError(err).Fatal("failed to create gcs cgroup")
cmd/gcs/main.go:353:		logrus.WithError(err).Fatal("failed add gcs pid to gcs cgroup")
cmd/gcs/main.go:359:		logrus.WithError(err).Fatal("failed to initialize new runc runtime")
cmd/gcs/main.go:392:		logrus.WithError(err).Fatal("failed to register memory threshold for gcs cgroup")
cmd/gcs/main.go:399:		logrus.WithError(err).Fatal("failed to retrieve the container cgroups oom eventfd")
cmd/gcs/main.go:407:		logrus.WithError(err).Fatal("failed to retrieve the virtual-pods cgroups oom eventfd")
cmd/gcs/main.go:415:			logrus.WithError(err).Fatal("failed to start time synchronization service")
cmd/ncproxy/ncproxy.go:108:			log.G(ctx).WithError(err).Warn("Failed to query ncproxy networking database")
cmd/ncproxy/ncproxy.go:279:			log.G(ctx).WithError(err).Warn("Failed to query ncproxy networking database")
cmd/ncproxy/ncproxy.go:459:			log.G(ctx).WithError(err).Warn("Failed to query ncproxy networking database")
cmd/ncproxy/ncproxy.go:511:			log.G(ctx).WithError(err).Warn("Failed to query ncproxy networking database")
cmd/ncproxy/ncproxy.go:546:			log.G(ctx).WithError(err).Warn("Failed to query ncproxy networking database")
cmd/ncproxy/ncproxy.go:612:		log.G(ctx).WithError(err).Warn("Failed to query ncproxy networking database")
cmd/ncproxy/ncproxy.go:691:		log.G(ctx).WithError(err).Warn("Failed to query ncproxy networking database")
cmd/ncproxy/ncproxy.go:812:		log.G(ctx).WithField("key", req.ContainerID).WithError(err).Warn("failed to delete key from compute agent store")
cmd/ncproxy/server.go:53:		log.G(ctx).WithError(err).Error("failed to create ttrpc server")
cmd/ncproxy/server.go:80:		log.G(ctx).WithError(err).Errorf("failed to listen on %s", s.conf.TTRPCAddr)
cmd/ncproxy/server.go:86:		log.G(ctx).WithError(err).Errorf("failed to listen on %s", s.conf.GRPCAddr)
cmd/ncproxy/server.go:96:		log.G(ctx).WithError(err).Error("failed to gracefully shutdown ttrpc server")
cmd/ncproxy/server.go:103:		log.G(ctx).WithError(err).Error("failed to disconnect connections in compute agent cache")
cmd/ncproxy/server.go:106:		log.G(ctx).WithError(err).Error("failed to close ncproxy compute agent database")
cmd/ncproxy/server.go:109:		log.G(ctx).WithError(err).Error("failed to close ncproxy networking database")
cmd/ncproxy/server.go:170:		log.G(ctx).WithError(err).Debug("no entries in database")
cmd/ncproxy/server.go:173:		log.G(ctx).WithError(err).Error("failed to get compute agent information")
cmd/ncproxy/server.go:184:				log.G(ctx).WithField("agentAddress", agentAddress).WithError(err).Error("failed to create new compute agent client")
cmd/ncproxy/server.go:187:					log.G(ctx).WithField("key", containerID).WithError(dErr).Warn("failed to delete key from compute agent store")
cmd/ncproxy/server.go:212:			log.G(ctx).WithError(err).Error("failed to close compute agent connection")
cmd/runhcs/vm.go:136:					logrus.WithError(err).
hcn/hcnnamespace.go:393:			WithError(err).
hcn/hcnsupport.go:81:		logrus.WithError(err).Errorf("unable to obtain supported features")
internal/cmd/cmd.go:228:				c.Log.WithError(err).Warn("failed to close Cmd stdin")
internal/cmd/cmd.go:237:				c.Log.WithError(cErr).Warn("failed to close Cmd stdout")
internal/cmd/cmd.go:247:				c.Log.WithError(cErr).Warn("failed to close Cmd stderr")
internal/cmd/cmd.go:278:		c.Log.WithError(waitErr).Warn("process wait failed")
internal/cmd/io.go:77:			log = log.WithError(err)
internal/cmd/io_binary.go:70:			log.G(ctx).WithError(err).Errorf("error closing wait pipe: %s", waitPipePath)
internal/cmd/io_binary.go:186:				log.G(ctx).WithError(err).Errorf("error while closing stdout npipe")
internal/cmd/io_binary.go:192:				log.G(ctx).WithError(err).Errorf("error while closing stderr npipe")
internal/cmd/io_binary.go:205:				log.G(ctx).WithError(err).Errorf("error while waiting for binary cmd to finish")
internal/cmd/io_binary.go:211:				log.G(ctx).WithError(err).Errorf("error while killing binaryIO process")
internal/cmd/io_binary.go:293:		log.G(context.TODO()).WithError(err).Debug("error closing pipe listener")
internal/cmd/io_npipe.go:263:			logrus.WithError(err).Error("failed to accept pipe")
internal/controller/vm/vm_wcow.go:54:			logrus.WithError(err).Error("failed to listen for windows logging connections")
internal/controller/vm/vm_wcow.go:71:				logrus.WithError(err).Error("failed to connect to log socket")
internal/devices/assigned_devices.go:37:				log.G(ctx).WithError(releaseErr).Error("failed to release container resource")
internal/devices/drivers.go:37:				log.G(ctx).WithError(releaseErr).Error("failed to release container resource")
internal/gcs-sidecar/bridge.go:189:		logrus.WithError(err).Errorf("error reading message header")
internal/gcs-sidecar/bridge.go:349:					log.G(ctx).WithError(err).Error("failed to send request to shimRequestChan")
internal/gcs-sidecar/bridge.go:374:					log.G(req.ctx).WithError(err).Errorf("failed to serve request: %v", req.header.Type.String())
internal/gcs-sidecar/bridge.go:386:						log.G(req.ctx).WithError(err).Errorf("failed to send response to shim")
internal/gcs-sidecar/bridge.go:455:						log.G(ctx).WithError(err).Error("failed to unmarshal the request")
internal/gcs-sidecar/bridge.go:473:					log.G(ctx).WithError(err).Error("failed to send request to b.sendToShimCh")
internal/gcs-sidecar/handlers.go:114:					log.G(ctx).WithError(err).Warn("Registry changes validation failed - rejecting")
internal/gcs-sidecar/handlers.go:150:					log.G(ctx).WithError(removeErr).Errorf("Failed to remove container: %v", containerID)
internal/gcs-sidecar/vsmb.go:49:				logrus.WithError(derr).Warn("Failed to disconnect Service Manager")
internal/gcs-sidecar/vsmb.go:62:				logrus.WithError(derr).Warn("Failed to close LanmanWorkstation service")
internal/gcs-sidecar/vsmb.go:146:				logrus.WithError(derr).Warn("Failed to disconnect Service Manager")
internal/gcs-sidecar/vsmb.go:158:				logrus.WithError(derr).Warn("Failed to close LanmanWorkstation service")
internal/gcs-sidecar/vsmb.go:198:		logrus.WithError(nerr).Errorf("invalid device name %q", GlobalRdrDeviceName)
internal/gcs-sidecar/vsmb.go:212:		logrus.WithError(err).Error("Failed to open redirector")
internal/gcs-sidecar/vsmb.go:217:			logrus.WithError(derr).Warn("Failed to close LanmanRedirector handle")
internal/gcs-sidecar/vsmb.go:226:		logrus.WithError(nerr).Errorf("invalid instance name %q", GlobalVsmbInstanceName)
internal/gcs-sidecar/vsmb.go:281:		logrus.WithError(nerr).Errorf("invalid device name %q", GlobalVsmbDeviceName)
internal/gcs-sidecar/vsmb.go:296:			logrus.WithError(derr).Warn("Failed to close VSMB device handle")
internal/gcs-sidecar/vsmb.go:302:		logrus.WithError(nerr).Errorf("invalid instance name %q", GlobalVsmbTransportName)
internal/gcs-sidecar/vsmb.go:342:		logrus.WithError(nerr).Errorf("invalid device name %q", device)
internal/gcs-sidecar/vsmb.go:356:		logrus.WithError(err).Errorf("Failed to open %s", device)
internal/gcs/bridge.go:100:			brdg.log.WithError(err).Warn("bridge error, already terminated")
internal/gcs/bridge.go:108:		brdg.log.WithError(err).Error("bridge forcibly terminating")
internal/gcs/bridge.go:404:			brdg.log.WithError(err).Warning("could not scrub bridge payload")
internal/gcs/bridge.go:446:			brdg.log.WithError(err).Error("bridge write failed but call is already complete")
internal/gcs/container.go:202:			log.G(ctx).WithError(err).Warn("ignoring missing container")
internal/gcs/guestconnection.go:323:					log.G(ctx).WithError(err).Warn("failed to encode OpenCensus Tracestate")
internal/gcs/process.go:135:		log.G(ctx).WithError(err).Warn("close stdin failed")
internal/gcs/process.go:138:		log.G(ctx).WithError(err).Warn("close stdout failed")
internal/gcs/process.go:141:		log.G(ctx).WithError(err).Warn("close stderr failed")
internal/gcs/process.go:290:		log.G(ctx).WithError(err).Error("failed wait")
internal/guest/bridge/bridge.go:325:						entry.WithError(err).Warning("could not scrub bridge payload")
internal/guest/kmsg/kmsg.go:111:		logrus.WithError(err).Error("failed to open /dev/kmsg")
internal/guest/kmsg/kmsg.go:129:			logrus.WithError(err).Error("kmsg read failure")
internal/guest/network/netns.go:160:				entry.WithError(err).Debugf("udhcpc failed [%s]", cos)
internal/guest/runtime/hcsv2/container.go:241:			entity.WithError(err).Error("failed to unmount sandbox mounts")
internal/guest/runtime/hcsv2/container.go:246:			entity.WithError(err).Error("failed to unmount tmpfs sandbox mounts")
internal/guest/runtime/hcsv2/container.go:251:			entity.WithError(err).Error("failed to unmount hugepages mounts")
internal/guest/runtime/hcsv2/process.go:99:			log.G(ctx).WithError(err).Error("failed to wait for runc process")
internal/guest/runtime/hcsv2/uvm.go:469:					log.G(ctx).WithError(err).Debug("failed to add SEV device")
internal/guest/runtime/hcsv2/uvm.go:663:			entry.WithError(err).Warning("could not scrub OCI spec written to config.json")
internal/guest/runtime/hcsv2/uvm.go:1550:				logrus.WithError(err).WithField("virtualSandboxID", virtualSandboxID).
internal/guest/runtime/hcsv2/uvm.go:1574:			logrus.WithError(err).WithField("virtualSandboxID", virtualSandboxID).
internal/guest/runtime/hcsv2/uvm.go:1584:				logrus.WithError(err).WithField("virtualSandboxID", virtualSandboxID).
internal/guest/runtime/runc/process.go:67:			l.WithError(err).Error("failed to terminate container after process wait")
internal/guest/spec/spec.go:479:			}).WithError(err).Warning("annotation value could not be parsed")
internal/guest/spec/spec_devices.go:137:				entry.WithError(err).Debugf("failed to find sysfs path for device %s", d.Path)
internal/guest/storage/crypt/crypt.go:51:			log.G(ctx).WithError(err).WithFields(logrus.Fields{
internal/guest/storage/crypt/crypt.go:59:						log.G(ctx).WithError(err).Warning("cryptsetup failed, context timeout")
internal/guest/storage/crypt/crypt.go:157:			log.G(ctx).WithError(err).Debugf("failed to delete temporary folder: %s", tempDir)
internal/guest/storage/crypt/crypt.go:180:				log.G(ctx).WithError(inErr).Debug("failed to cleanup crypt device")
internal/guest/storage/devicemapper/devicemapper.go:237:		log.G(ctx).WithError(err).Warning("CreateDevice error")
internal/guest/storage/devicemapper/devicemapper.go:248:					log.G(ctx).WithError(err).Error("CreateDeviceWithRetryErrors failed, context timeout")
internal/guest/storage/overlay/overlay.go:36:		log.G(ctx).WithError(statErr).WithField("path", filepath.Dir(path)).Warn("failed to get disk information for ENOSPC error")
internal/guest/storage/overlay/overlay.go:56:	}).WithError(err).Warn("got ENOSPC, gathering diagnostics")
internal/guest/storage/pmem/pmem.go:47:				log.G(ctx).WithError(err).Debugf("error cleaning up target: %s", target)
internal/guest/storage/pmem/pmem.go:104:					log.G(mCtx).WithError(err).Debugf("failed to cleanup linear target: %s", dmLinearName)
internal/guest/storage/pmem/pmem.go:118:					log.G(mCtx).WithError(err).Debugf("failed to cleanup verity target: %s", dmVerityName)
internal/guest/storage/pmem/pmem.go:151:			log.G(ctx).WithError(err).Debugf("failed to remove dm verity target: %s", dmVerityName)
internal/guest/storage/pmem/pmem.go:159:			log.G(ctx).WithError(err).Debugf("failed to remove dm linear target: %s", dmLinearName)
internal/guest/storage/scsi/scsi.go:163:						log.G(spnCtx).WithError(err).WithField("verityTarget", dmVerityName).Debug("failed to cleanup verity target")
internal/guest/storage/scsi/scsi.go:222:			log.G(ctx).WithError(err).Debug("get device filesystem failed, retrying in 500ms")
internal/guest/storage/scsi/scsi.go:318:			log.G(ctx).WithError(err).Debugf("failed to remove dm verity target: %s", dmVerityName)
internal/guest/storage/scsi/scsi.go:432:			log.G(ctx).WithError(err).Warnf("failed to close file: %s", devicePath)
internal/hcs/errors.go:136:			log.G(ctx).WithError(err).Warning("Could not unmarshal HCS result")
internal/hcs/process.go:77:					log.G(ctx).WithError(err).Warn("force unblocking process waits")
internal/hcs/process.go:230:		log.G(ctx).WithError(err).Error("failed wait")
internal/hcs/system.go:414:					log.G(ctx).WithError(err).Warn("failed to get statistics in-proc")
internal/hcs/system.go:549:		logEntry = logEntry.WithError(fmt.Errorf("failed to query compute system properties in-proc: %w", err))
internal/hcsoci/create.go:252:			log.G(ctx).WithError(err).Debug("failed to allocateLinuxResources")
internal/hcsoci/create.go:257:			log.G(ctx).WithError(err).Debug("failed createHCSContainerDocument")
internal/hcsoci/create.go:263:			log.G(ctx).WithError(err).Debug("failed to allocateWindowsResources")
internal/hcsoci/create.go:269:			log.G(ctx).WithError(err).Debug("failed createHCSContainerDocument")
internal/hcsoci/devices.go:110:					log.G(ctx).WithError(releaseErr).Error("failed to release container resource")
internal/hcsoci/devices.go:167:					log.G(ctx).WithError(releaseErr).Error("failed to release container resource")
internal/hcsoci/devices.go:204:					log.G(ctx).WithError(releaseErr).Error("failed to release container resource")
internal/hvsocket/hvsocket.go:97:			log.G(context.Background()).WithError(closeErr).Debug("failed to close address info handle")
internal/jobcontainers/jobcontainer.go:441:		log.G(context.Background()).WithError(err).WithField("cid", c.id).Warning("failed to close job object")
internal/jobcontainers/jobcontainer.go:446:		log.G(context.Background()).WithError(err).WithField("cid", c.id).Warning("failed to close token")
internal/jobcontainers/jobcontainer.go:453:			log.G(context.Background()).WithError(err).WithField("cid", c.id).Warning("failed to delete local account")
internal/jobcontainers/jobcontainer.go:657:			log.G(ctx).WithError(err).Warn("error while polling for job container notification")
internal/jobcontainers/mounts.go:133:			log.G(ctx).WithError(err).Warnf("failed to setup symlink from %s to containers rootfs at %s", mount.Source, fullCtrPath)
internal/jobcontainers/storage.go:54:					log.G(ctx).WithError(closeErr).Errorf("failed to cleanup mounted layers during another failure(%s)", err)
internal/jobobject/iocp.go:46:			log.G(ctx).WithError(err).Error("failed to poll for job object message")
internal/layers/lcow.go:51:		log.G(ctx).WithError(err).Error("failed RemoveCombinedLayersLCOW")
internal/layers/lcow.go:57:		log.G(ctx).WithError(err).Error("failed LCOW scratch mount release")
internal/layers/lcow.go:107:					log.G(ctx).WithError(err).Warn("failed to remove lcow layer on cleanup")
internal/layers/lcow.go:169:				log.G(ctx).WithError(err).Warn("failed to remove scratch on cleanup")
internal/layers/wcow_mount.go:165:					log.G(ctx).WithField("path", l.scratchLayerPath).WithError(hcserr.Err).Warning("retrying layer operations after failure")
internal/layers/wcow_mount.go:236:				log.G(ctx).WithError(err).Warnf("mount process isolated cim layers common, undo failed with: %s", rErr)
internal/layers/wcow_mount.go:303:				log.G(ctx).WithError(err).Warnf("mount process isolated forked CIM layers, undo failed with: %s", rErr)
internal/layers/wcow_mount.go:336:				log.G(ctx).WithError(err).Warnf("mount process isolated forked CIM layers, undo failed with: %s", rErr)
internal/layers/wcow_mount.go:379:		log.G(ctx).WithError(err).Error("failed RemoveCombinedLayersWCOW")
internal/layers/wcow_mount.go:385:		log.G(ctx).WithError(err).Error("failed WCOW scratch mount release")
internal/layers/wcow_mount.go:421:					log.G(ctx).WithError(err).Warn("failed to remove wcow layer on cleanup")
internal/layers/wcow_mount.go:451:				log.G(ctx).WithError(err).Warn("failed to remove scratch on cleanup")
internal/layers/wcow_mount.go:507:				log.G(ctx).WithError(err).Warnf("mount process isolated forked CIM layers, undo failed with: %s", rErr)
internal/lcow/scratch.go:108:		log.G(ctx).WithError(err).WithField("stderr", mkfsStderr.String()).Error("mkfs.ext4 failed")
internal/oci/annotations.go:51:					}).WithError(err).Warning("annotation expansion would overwrite conflicting value")
internal/oci/annotations.go:74:		}).WithError(err).Warning("Host process container and disable host process container cannot both be true")
internal/oci/annotations.go:238:			entry.WithError(err).Warn("invalid GUID string for Hyper-V socket service configuration annotation")
internal/oci/annotations.go:411:		entry = entry.WithError(err)
internal/resources/resources.go:137:					log.G(ctx).WithError(err).Error("failed to release container resource")
internal/resources/resources.go:144:				log.G(ctx).WithError(err).Error("failed to release container resource")
internal/resources/resources.go:153:					log.G(ctx).WithError(err).Error("failed to release container resource")
internal/shim/publisher.go:109:			log.L.WithError(err).Error("forward event")
internal/shim/shim.go:466:			log.G(ctx).WithError(err).Fatal("containerd-shim: ttrpc server failure")
internal/shim/shim.go:481:			log.G(ctx).WithError(err).Warn("Could not setup pprof")
internal/shim/shim.go:527:			log.G(ctx).WithError(err).Fatal("containerd-shim: pprof endpoint failure")
internal/tools/networkagent/main.go:468:		log.G(ctx).WithError(err).Fatalf("failed to read network agent's config file at %s", *configPath)
internal/tools/networkagent/main.go:488:		log.G(ctx).WithError(err).Fatalf("failed to connect to ncproxy at %s", conf.GRPCAddr)
internal/tools/networkagent/main.go:510:		log.G(ctx).WithError(err).Fatalf("failed to listen on %s", grpcListener.Addr().String())
internal/tools/networkagent/main.go:531:			log.G(ctx).WithError(err).Fatal("grpc service failure")
internal/tools/rootfs/merge.go:403:		entry.WithError(err).Warn("unable to stat")
internal/tools/uvmboot/lcow.go:364:			log.G(ctx).WithError(err).Warn("could not create console from stdin")
internal/tools/uvmboot/main.go:160:					logrus.WithField("uvm-id", id).WithError(err).Error("failed to run UVM")
internal/tools/uvmboot/mounts.go:102:			entry.WithError(err).Warnf("invald %s flag value", name)
internal/uvm/computeagent.go:270:			log.G(ctx).WithError(err).Fatal("compute agent: serve failure")
internal/uvm/create.go:340:			e.WithError(err).Error("failed to remove VMGS file")
internal/uvm/create_lcow.go:740:							log.G(ctx).WithError(err).Debug("failed to release memory region")
internal/uvm/modify.go:32:					log.G(ctx).WithError(rerr).Error("failed to roll back resource add")
internal/uvm/modify.go:45:			log.G(ctx).WithError(err).Error("failed to remove host resources after successful guest request")
internal/uvm/start.go:92:				e.WithError(err).Error("failed to connect to entropy socket")
internal/uvm/start.go:98:				e.WithError(err).Error("failed to write entropy")
internal/uvm/start.go:121:						e.WithError(err).Error("failed to connect to log socket")
internal/uvm/start.go:143:					e.WithError(err).Error("failed to connect to log socket")
internal/uvm/start.go:293:			e.WithError(err).Error("failed to set log sources")
internal/uvm/start.go:296:			e.WithError(err).Error("failed to start log forwarding")
internal/uvm/vpmem_mapped.go:250:				log.G(ctx).WithError(err).Debugf("failed to reclaim pmem region: %s", err)
internal/uvm/vpmem_mapped.go:265:				log.G(ctx).WithError(err).Debugf("failed to rollback modification")
internal/uvm/vpmem_mapped.go:312:		log.G(ctx).WithError(err).Debugf("failed unmapping VHD layer %s", hostPath)
internal/vm/vmutils/vmmem.go:35:			log.G(ctx).WithError(err).Error("failed to create process snapshot")
internal/vm/vmutils/vmmem.go:47:				log.G(ctx).WithError(err).Debug("finished iterating process entries")
internal/wclayer/cim/block_cim_writer.go:64:				log.G(ctx).WithError(err).Warnf("failed to close cim after error: %s", cErr)
internal/wclayer/cim/forked_cim_writer.go:45:				log.G(ctx).WithError(err).Warnf("failed to close cim after error: %s", cErr)
internal/wclayer/cim/forked_cim_writer.go:49:				log.G(ctx).WithError(err).Warnf("failed to cleanup cim after error: %s", cErr)
internal/wclayer/layerutils.go:89:			logrus.WithError(err).Debug("Failed to convert name to guid")
internal/wclayer/layerutils.go:95:			logrus.WithError(err).Debug("Failed conversion of parentLayerPath to pointer")
internal/winapi/cimwriter/cimwriter.go:49:			logrus.WithError(freeErr).Warn("failed to free cimwriter.dll after load failure")
pkg/cimfs/cim_writer_windows.go:365:		log.G(ctx).WithError(err).Warnf("get region files for cim %s", cimPath)
pkg/cimfs/cim_writer_windows.go:372:		log.G(ctx).WithError(err).Warnf("get objectid file for cim %s", cimPath)
pkg/cimfs/cim_writer_windows.go:386:			log.G(ctx).WithError(err).Warnf("remove file %s", regFilePath)
pkg/cimfs/cim_writer_windows.go:395:			log.G(ctx).WithError(err).Warnf("remove file %s", objFilePath)
pkg/cimfs/cim_writer_windows.go:403:		log.G(ctx).WithError(err).Warnf("remove file %s", cimPath)
pkg/cimfs/cimfs.go:19:		logrus.WithError(err).Warn("get build revision")
pkg/cimfs/common.go:108:			log.G(ctx).WithError(err).Warnf("stat for object file %s", path)
pkg/cimfs/common.go:134:			log.G(ctx).WithError(err).Warnf("stat for region file %s", path)
pkg/ociwclayer/cim/import.go:284:				log.G(ctx).WithError(flushErr).Warn("flush buffer during layer write failed")
pkg/ociwclayer/cim/import.go:335:				log.G(ctx).WithError(retErr).Warnf("error in cleanup on failure: %s", rmErr)
pkg/securitypolicy/securitypolicyenforcer_rego.go:270:		return nil, policy.denyWithError(ctx, err, input)
pkg/securitypolicy/securitypolicyenforcer_rego.go:275:		return result, policy.denyWithError(ctx, err, input)
pkg/securitypolicy/securitypolicyenforcer_rego.go:280:		return nil, policy.denyWithError(ctx, err, input)
pkg/securitypolicy/securitypolicyenforcer_rego.go:322:		log.G(ctx).WithError(err).Error("unable to marshal error object")
pkg/securitypolicy/securitypolicyenforcer_rego.go:346:			log.G(ctx).WithError(err).Error("unable to marshal error object")
pkg/securitypolicy/securitypolicyenforcer_rego.go:360:func (policy *regoEnforcer) denyWithError(ctx context.Context, policyError error, input inputData) error {
pkg/securitypolicy/securitypolicyenforcer_rego.go:390:		log.G(ctx).WithError(err).Warn("unable to obtain reason for policy decision")
pkg/securitypolicy/securitypolicyenforcer_rego.go:763:			log.G(ctx).WithError(err).Warn("failed to obtain policy metadata snapshot")
test/functional/main_test.go:245:					logrus.WithError(err).Error("could not create ETW logrus hook")
test/gcs/main_test.go:66:		logrus.WithError(err).Fatal("could not set up testing")
test/internal/layers/lazy.go:292:		}).WithError(err).Warning("failed to add MS defender exclusion for image layers directory")
vendor/github.com/Microsoft/cosesign1go/pkg/cosesign1/create.go:75:		logrus.WithError(err).Debug("failed to decode a key")
vendor/github.com/Microsoft/cosesign1go/pkg/cosesign1/create.go:90:		logrus.WithError(err).Debug("cert parsing failed")
vendor/github.com/Microsoft/cosesign1go/pkg/cosesign1/create.go:114:		logrus.WithError(err).Error("failed to initialize cose signer")
vendor/github.com/Microsoft/cosesign1go/pkg/cosesign1/create.go:138:		logrus.WithError(err).Debug("failed to create cose.Sign1")
vendor/github.com/containerd/containerd/v2/core/mount/mount_linux.go:258:				log.L.WithError(err).Warnf("failed to unmount temp lowerdir %s", lowerDir)
vendor/github.com/containerd/containerd/v2/core/mount/mount_linux.go:264:				log.L.WithError(err).Warnf("failed to remove temporary overlay lowerdir")
vendor/github.com/containerd/containerd/v2/core/mount/mount_linux.go:270:			log.L.WithError(err).Infof("failed to remove temporary overlay dir")
vendor/github.com/containerd/containerd/v2/core/mount/mount_windows.go:59:				log.G(context.TODO()).WithError(layerErr).Error("failed to deactivate layer during mount failure cleanup")
vendor/github.com/containerd/containerd/v2/core/mount/mount_windows.go:71:				log.G(context.TODO()).WithError(layerErr).Error("failed to unprepare layer during mount failure cleanup")
vendor/github.com/containerd/containerd/v2/core/mount/mount_windows.go:95:				log.G(context.TODO()).WithError(bindErr).Error("failed to remove binding during mount failure cleanup")
vendor/github.com/containerd/containerd/v2/core/mount/temp.go:50:			log.G(ctx).WithError(uerr).WithField("dir", root).Error("failed to remove mount temp dir")
vendor/github.com/containerd/containerd/v2/pkg/shim/publisher.go:106:			log.L.WithError(err).Error("forward event")
vendor/github.com/containerd/containerd/v2/pkg/shim/shim.go:459:			log.G(ctx).WithError(err).Fatal("containerd-shim: ttrpc server failure")
vendor/github.com/containerd/containerd/v2/pkg/shim/shim.go:465:			log.G(ctx).WithError(err).Warn("Could not setup pprof")
vendor/github.com/containerd/containerd/v2/pkg/shim/shim.go:511:			log.G(ctx).WithError(err).Fatal("containerd-shim: pprof endpoint failure")
vendor/github.com/containerd/containerd/v2/pkg/shim/shim_unix.go:86:					logger.WithError(err).Error("reap exit status")
vendor/github.com/containerd/containerd/v2/pkg/sys/pidfd_linux.go:41:			logger.WithError(err).Error("failed to ensure the kernel supports pidfd")
vendor/github.com/containerd/go-runc/io_unix.go:56:				logrus.WithError(err).Debug("failed to chown stdin, ignored")
vendor/github.com/containerd/go-runc/io_unix.go:71:				logrus.WithError(err).Debug("failed to chown stdout, ignored")
vendor/github.com/containerd/go-runc/io_unix.go:86:				logrus.WithError(err).Debug("failed to chown stderr, ignored")
vendor/github.com/containerd/stargz-snapshotter/estargz/build.go:258:			pw.CloseWithError(err)
vendor/github.com/containerd/stargz-snapshotter/estargz/build.go:263:			pw.CloseWithError(err)
vendor/github.com/containerd/stargz-snapshotter/estargz/build.go:411:				pw.CloseWithError(fmt.Errorf("Failed to write tar header: %v", err))
vendor/github.com/containerd/stargz-snapshotter/estargz/build.go:415:				pw.CloseWithError(fmt.Errorf("Failed to write tar payload: %v", err))
vendor/github.com/containerd/ttrpc/client.go:376:				s.closeWithError(err)
vendor/github.com/containerd/ttrpc/client.go:439:	s.closeWithError(nil)
vendor/github.com/containerd/ttrpc/client.go:454:		s.closeWithError(err)
vendor/github.com/containerd/ttrpc/server.go:121:				log.G(ctx).WithError(err).Errorf("ttrpc: failed accept; backoff %v", sleep)
vendor/github.com/containerd/ttrpc/server.go:133:			log.G(ctx).WithError(err).Error("ttrpc: refusing connection after handshake")
vendor/github.com/containerd/ttrpc/server.go:140:			log.G(ctx).WithError(err).Error("ttrpc: create connection failed")
vendor/github.com/containerd/ttrpc/server.go:523:					log.G(ctx).WithError(err).Error("failed marshaling response")
vendor/github.com/containerd/ttrpc/server.go:528:					log.G(ctx).WithError(err).Error("failed sending message on channel")
vendor/github.com/containerd/ttrpc/server.go:540:					log.G(ctx).WithError(err).Error("failed sending message on channel")
vendor/github.com/containerd/ttrpc/server.go:562:			log.G(ctx).WithError(err).Error("error receiving message")
vendor/github.com/containerd/ttrpc/stream.go:50:func (s *stream) closeWithError(err error) error {
vendor/github.com/docker/cli/cli/config/configfile/file.go:183:				logrus.WithError(err).WithField("file", temp.Name()).Debug("Error cleaning up temp file")
vendor/github.com/docker/cli/cli/config/configfile/file.go:394:			logrus.WithError(err).Warnf("Failed to get credentials for registry: %s", registryHostname)
vendor/github.com/godbus/dbus/v5/conn.go:397:				conn.calls.finalizeAllWithError(sequenceGen, err)
vendor/github.com/godbus/dbus/v5/conn.go:927:		tracker.finalizeWithError(serial, sequence, Error{name, msg.Body})
vendor/github.com/godbus/dbus/v5/conn.go:940:		tracker.finalizeWithError(msg.serial, NoSequence, err)
vendor/github.com/godbus/dbus/v5/conn.go:969:func (tracker *callTracker) finalizeWithError(sn uint32, sequence Sequence, err error) {
vendor/github.com/godbus/dbus/v5/conn.go:983:func (tracker *callTracker) finalizeAllWithError(sequenceGen *sequenceGenerator, err error) {
vendor/github.com/google/go-containerregistry/internal/gzip/zip.go:52:	// Returns err so we can pw.CloseWithError(err)
vendor/github.com/google/go-containerregistry/internal/gzip/zip.go:58:			return pw.CloseWithError(err)
vendor/github.com/google/go-containerregistry/internal/gzip/zip.go:64:			return pw.CloseWithError(err)
vendor/github.com/google/go-containerregistry/internal/gzip/zip.go:69:			return pw.CloseWithError(err)
vendor/github.com/google/go-containerregistry/internal/gzip/zip.go:74:			return pw.CloseWithError(err)
vendor/github.com/google/go-containerregistry/internal/zstd/zstd.go:50:	// Returns err so we can pw.CloseWithError(err)
vendor/github.com/google/go-containerregistry/internal/zstd/zstd.go:56:			return pw.CloseWithError(err)
vendor/github.com/google/go-containerregistry/internal/zstd/zstd.go:62:			return pw.CloseWithError(err)
vendor/github.com/google/go-containerregistry/internal/zstd/zstd.go:67:			return pw.CloseWithError(err)
vendor/github.com/google/go-containerregistry/internal/zstd/zstd.go:72:			return pw.CloseWithError(err)
vendor/github.com/google/go-containerregistry/pkg/v1/mutate/mutate.go:257:		pw.CloseWithError(extract(img, pw))
vendor/github.com/google/go-containerregistry/pkg/v1/stream/layer.go:237:			pw.CloseWithError(copyErr)
vendor/github.com/google/go-containerregistry/pkg/v1/stream/layer.go:242:			pw.CloseWithError(closeErr)
vendor/github.com/google/go-containerregistry/pkg/v1/stream/layer.go:249:			pw.CloseWithError(err)
vendor/github.com/google/go-containerregistry/pkg/v1/stream/layer.go:259:		pw.CloseWithError(cr.Close())
vendor/github.com/opencontainers/cgroups/systemd/common.go:254:			logrus.WithError(err).Error("unable to get systemd version")
vendor/github.com/opencontainers/cgroups/utils.go:62:				logrus.WithError(err).Debugf("statfs(%q) failed", hybridMountpoint)
vendor/github.com/opencontainers/runc/libcontainer/configs/validate/validator.go:48:			logrus.WithError(err).Warn("configuration")
vendor/github.com/opencontainers/runc/libcontainer/container_linux.go:397:				logrus.WithError(err).Warn("failed to kill all processes, possibly due to lack of cgroup (Hint: enable cgroup v2 delegation)")
vendor/github.com/opencontainers/runc/libcontainer/env.go:78:			logrus.WithError(err).Debugf("HOME not set in process.env, and getting UID %d homedir failed", uid)
vendor/github.com/opencontainers/runc/libcontainer/exeseal/cloned_binary_linux.go:235:	logrus.WithError(err).Debugf("could not use overlayfs for /proc/self/exe sealing -- falling back to making a temporary copy")
vendor/github.com/opencontainers/runc/libcontainer/process_linux.go:200:		logrus.WithError(
vendor/github.com/opencontainers/runc/libcontainer/process_linux.go:271:				logrus.WithError(err).Warn("unable to terminate setnsProcess")
vendor/github.com/opencontainers/runc/libcontainer/process_linux.go:624:				logrus.WithError(err).Warn("unable to get oom kill count")
vendor/github.com/opencontainers/runc/libcontainer/process_linux.go:641:				logrus.WithError(err).Warn("unable to terminate initProcess")
vendor/github.com/samber/lo/README.md:357:- [TryWithErrorValue](#trywitherrorvalue)
vendor/github.com/samber/lo/README.md:358:- [TryCatchWithErrorValue](#trycatchwitherrorvalue)
vendor/github.com/samber/lo/README.md:4321:### TryWithErrorValue
vendor/github.com/samber/lo/README.md:4326:err, ok := lo.TryWithErrorValue(func() error {
vendor/github.com/samber/lo/README.md:4354:### TryCatchWithErrorValue
vendor/github.com/samber/lo/README.md:4356:The same behavior as `TryWithErrorValue`, but calls the catch function in case of error.
vendor/github.com/samber/lo/README.md:4361:ok := lo.TryCatchWithErrorValue(func() error {
vendor/github.com/samber/lo/errors.go:312:// TryWithErrorValue has the same behavior as Try, but also returns value passed to panic.
vendor/github.com/samber/lo/errors.go:314:func TryWithErrorValue(callback func() error) (errorValue any, ok bool) {
vendor/github.com/samber/lo/errors.go:341:// TryCatchWithErrorValue has the same behavior as TryWithErrorValue, but calls the catch function in case of error.
vendor/github.com/samber/lo/errors.go:343:func TryCatchWithErrorValue(callback func() error, catch func(any)) {
vendor/github.com/samber/lo/errors.go:344:	if err, ok := TryWithErrorValue(callback); !ok {
vendor/github.com/sirupsen/logrus/CHANGELOG.md:210:* logrus/core: support `WithError` on logger
vendor/github.com/sirupsen/logrus/entry.go:37:// Defines the key when adding errors using WithError.
vendor/github.com/sirupsen/logrus/entry.go:106:func (entry *Entry) WithError(err error) *Entry {
vendor/github.com/sirupsen/logrus/exported.go:54:// WithError creates an entry from the standard logger and adds an error to it, using the value defined in ErrorKey as key.
vendor/github.com/sirupsen/logrus/exported.go:55:func WithError(err error) *Entry {
vendor/github.com/sirupsen/logrus/logger.go:130:// `WithError` for the given `error`.
vendor/github.com/sirupsen/logrus/logger.go:131:func (logger *Logger) WithError(err error) *Entry {
vendor/github.com/sirupsen/logrus/logger.go:134:	return entry.WithError(err)
vendor/github.com/sirupsen/logrus/logrus.go:142:	WithError(err error) *Entry
vendor/github.com/urfave/cli/flag.go:106:	ApplyWithError(*flag.FlagSet) error
vendor/github.com/urfave/cli/flag.go:115:			if err := ef.ApplyWithError(set); err != nil {
vendor/github.com/urfave/cli/flag_bool.go:70:	_ = f.ApplyWithError(set)
vendor/github.com/urfave/cli/flag_bool.go:73:// ApplyWithError populates the flag given the flag set and environment
vendor/github.com/urfave/cli/flag_bool.go:74:func (f BoolFlag) ApplyWithError(set *flag.FlagSet) error {
vendor/github.com/urfave/cli/flag_bool_t.go:70:	_ = f.ApplyWithError(set)
vendor/github.com/urfave/cli/flag_bool_t.go:73:// ApplyWithError populates the flag given the flag set and environment
vendor/github.com/urfave/cli/flag_bool_t.go:74:func (f BoolTFlag) ApplyWithError(set *flag.FlagSet) error {
vendor/github.com/urfave/cli/flag_duration.go:71:	_ = f.ApplyWithError(set)
vendor/github.com/urfave/cli/flag_duration.go:74:// ApplyWithError populates the flag given the flag set and environment
vendor/github.com/urfave/cli/flag_duration.go:75:func (f DurationFlag) ApplyWithError(set *flag.FlagSet) error {
vendor/github.com/urfave/cli/flag_float64.go:71:	_ = f.ApplyWithError(set)
vendor/github.com/urfave/cli/flag_float64.go:74:// ApplyWithError populates the flag given the flag set and environment
vendor/github.com/urfave/cli/flag_float64.go:75:func (f Float64Flag) ApplyWithError(set *flag.FlagSet) error {
vendor/github.com/urfave/cli/flag_generic.go:65:	_ = f.ApplyWithError(set)
vendor/github.com/urfave/cli/flag_generic.go:68:// ApplyWithError takes the flagset and calls Set on the generic flag with the value
vendor/github.com/urfave/cli/flag_generic.go:70:func (f GenericFlag) ApplyWithError(set *flag.FlagSet) error {
vendor/github.com/urfave/cli/flag_int.go:56:	_ = f.ApplyWithError(set)
vendor/github.com/urfave/cli/flag_int.go:59:// ApplyWithError populates the flag given the flag set and environment
vendor/github.com/urfave/cli/flag_int.go:60:func (f IntFlag) ApplyWithError(set *flag.FlagSet) error {
vendor/github.com/urfave/cli/flag_int64.go:56:	_ = f.ApplyWithError(set)
vendor/github.com/urfave/cli/flag_int64.go:59:// ApplyWithError populates the flag given the flag set and environment
vendor/github.com/urfave/cli/flag_int64.go:60:func (f Int64Flag) ApplyWithError(set *flag.FlagSet) error {
vendor/github.com/urfave/cli/flag_int64_slice.go:92:	_ = f.ApplyWithError(set)
vendor/github.com/urfave/cli/flag_int64_slice.go:95:// ApplyWithError populates the flag given the flag set and environment
vendor/github.com/urfave/cli/flag_int64_slice.go:96:func (f Int64SliceFlag) ApplyWithError(set *flag.FlagSet) error {
vendor/github.com/urfave/cli/flag_int_slice.go:92:	_ = f.ApplyWithError(set)
vendor/github.com/urfave/cli/flag_int_slice.go:95:// ApplyWithError populates the flag given the flag set and environment
vendor/github.com/urfave/cli/flag_int_slice.go:96:func (f IntSliceFlag) ApplyWithError(set *flag.FlagSet) error {
vendor/github.com/urfave/cli/flag_string.go:53:	_ = f.ApplyWithError(set)
vendor/github.com/urfave/cli/flag_string.go:56:// ApplyWithError populates the flag given the flag set and environment
vendor/github.com/urfave/cli/flag_string.go:57:func (f StringFlag) ApplyWithError(set *flag.FlagSet) error {
vendor/github.com/urfave/cli/flag_string_slice.go:83:	_ = f.ApplyWithError(set)
vendor/github.com/urfave/cli/flag_string_slice.go:86:// ApplyWithError populates the flag given the flag set and environment
vendor/github.com/urfave/cli/flag_string_slice.go:87:func (f StringSliceFlag) ApplyWithError(set *flag.FlagSet) error {
vendor/github.com/urfave/cli/flag_uint.go:50:	_ = f.ApplyWithError(set)
vendor/github.com/urfave/cli/flag_uint.go:53:// ApplyWithError populates the flag given the flag set and environment
vendor/github.com/urfave/cli/flag_uint.go:54:func (f UintFlag) ApplyWithError(set *flag.FlagSet) error {
vendor/github.com/urfave/cli/flag_uint64.go:56:	_ = f.ApplyWithError(set)
vendor/github.com/urfave/cli/flag_uint64.go:59:// ApplyWithError populates the flag given the flag set and environment
vendor/github.com/urfave/cli/flag_uint64.go:60:func (f Uint64Flag) ApplyWithError(set *flag.FlagSet) error {
vendor/golang.org/x/net/http2/pipe.go:106:// CloseWithError causes the next Read (waking up a current blocked
vendor/golang.org/x/net/http2/pipe.go:111:func (p *pipe) CloseWithError(err error) { p.closeWithError(&p.err, err, nil) }
vendor/golang.org/x/net/http2/pipe.go:113:// BreakWithError causes the next Read (waking up a current blocked
vendor/golang.org/x/net/http2/pipe.go:116:func (p *pipe) BreakWithError(err error) { p.closeWithError(&p.breakErr, err, nil) }
vendor/golang.org/x/net/http2/pipe.go:118:// closeWithErrorAndCode is like CloseWithError but also sets some code to run
vendor/golang.org/x/net/http2/pipe.go:120:func (p *pipe) closeWithErrorAndCode(err error, fn func()) { p.closeWithError(&p.err, err, fn) }
vendor/golang.org/x/net/http2/pipe.go:122:func (p *pipe) closeWithError(dst *error, err error, fn func()) {
vendor/golang.org/x/net/http2/pipe.go:161:// Err returns the error (if any) first set by BreakWithError or CloseWithError.
vendor/golang.org/x/net/http2/pipe.go:172:// with CloseWithError.
vendor/golang.org/x/net/http2/server.go:1739:		p.CloseWithError(err)
vendor/golang.org/x/net/http2/server.go:1900:		st.body.CloseWithError(fmt.Errorf("sender tried to send more than declared Content-Length of %d bytes", st.declBodyBytes))
vendor/golang.org/x/net/http2/server.go:1968:		st.body.CloseWithError(fmt.Errorf("request declared a Content-Length of %d but only wrote %d bytes",
vendor/golang.org/x/net/http2/server.go:1971:		st.body.closeWithErrorAndCode(io.EOF, st.copyTrailersToHandlerRequest)
vendor/golang.org/x/net/http2/server.go:1972:		st.body.CloseWithError(io.EOF)
vendor/golang.org/x/net/http2/server.go:1994:		st.body.CloseWithError(fmt.Errorf("%w", os.ErrDeadlineExceeded))
vendor/golang.org/x/net/http2/server.go:2549:			b.pipe.BreakWithError(errClosedBody)
vendor/golang.org/x/net/http2/transport.go:1726:		cs.bufPipe.CloseWithError(err) // no-op if already closed
vendor/golang.org/x/net/http2/transport.go:1731:		cs.bufPipe.CloseWithError(errRequestCanceled)
vendor/golang.org/x/net/http2/transport.go:2611:	cs.bufPipe.BreakWithError(errClosedResponseBody)
vendor/golang.org/x/net/http2/transport.go:2772:		cs.bufPipe.closeWithErrorAndCode(io.EOF, cs.copyTrailers)
vendor/golang.org/x/net/http2/transport.go:2978:	cs.bufPipe.CloseWithError(serr)
vendor/golang.org/x/tools/internal/stdlib/manifest.go:7076:		{"(*PipeReader).CloseWithError", Method, 0, ""},
vendor/golang.org/x/tools/internal/stdlib/manifest.go:7079:		{"(*PipeWriter).CloseWithError", Method, 0, ""},
```

## logrus.Fields usages

```txt
cmd/containerd-shim-runhcs-v1/exec_hcs.go:46:	log.G(ctx).WithFields(logrus.Fields{
cmd/containerd-shim-runhcs-v1/exec_hcs.go:205:		Log: log.G(ctx).WithFields(logrus.Fields{
cmd/containerd-shim-runhcs-v1/exec_wcow_podsandbox.go:22:	log.G(ctx).WithFields(logrus.Fields{
cmd/containerd-shim-runhcs-v1/task_hcs.go:191:	log.G(ctx).WithFields(logrus.Fields{
cmd/containerd-shim-runhcs-v1/task_hcs.go:436:				log.G(ctx).WithFields(logrus.Fields{
cmd/containerd-shim-runhcs-v1/task_hcs.go:773:	ctx, _ = log.SetEntry(ctx, logrus.Fields{logfields.UVMID: ht.host.ID()})
cmd/containerd-shim-runhcs-v1/task_wcow_podsandbox.go:260:	ctx, _ = log.SetEntry(ctx, logrus.Fields{logfields.UVMID: wpst.host.ID()})
cmd/gcs-sidecar/main.go:218:	logrus.WithFields(logrus.Fields{
cmd/gcs/main.go:37:func memoryLogFormat(metrics *cgroupstats.Metrics) logrus.Fields {
cmd/gcs/main.go:38:	return logrus.Fields{
cmd/gcs/main.go:77:		entry := logrus.WithFields(logrus.Fields{
cmd/gcs/main.go:111:			logrus.WithFields(logrus.Fields{
cmd/gcs/main.go:231:			logrus.WithFields(logrus.Fields{
cmd/gcs/main.go:252:		logrus.WithFields(logrus.Fields{
cmd/gcs/main.go:265:		logrus.WithFields(logrus.Fields{
cmd/gcs/main.go:281:	logrus.WithFields(logrus.Fields{
cmd/gcs/main.go:380:			logrus.WithFields(logrus.Fields{
cmd/gcs/main.go:424:		logrus.WithFields(logrus.Fields{
cmd/ncproxy/run.go:277:	log.G(ctx).WithFields(logrus.Fields{
cmd/ncproxy/server.go:122:		log.G(ctx).WithFields(logrus.Fields{
cmd/ncproxy/server.go:131:		log.G(ctx).WithFields(logrus.Fields{
cmd/runhcs/container.go:493:			logrus.WithFields(logrus.Fields{
cmd/runhcs/container.go:524:	logrus.WithFields(logrus.Fields{
cmd/runhcs/vm.go:152:	logrus.WithFields(logrus.Fields{
hcn/hcnsupport.go:124:	log.L.WithFields(logrus.Fields{
internal/bridgeutils/commonutils/utilities.go:59:			logrus.WithFields(logrus.Fields{
internal/builder/vm/lcow/boot.go:38:		log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/boot.go:92:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/boot.go:165:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/confidential.go:141:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/confidential.go:177:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/confidential.go:189:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/confidential.go:228:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/devices.go:38:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/devices.go:61:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/devices.go:96:			log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/devices.go:131:		log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/devices.go:187:				log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/devices.go:204:		log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/devices.go:215:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/kernel_args.go:100:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/kernel_args.go:161:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/specs.go:338:		log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/specs.go:359:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/topology.go:74:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/topology.go:116:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/topology.go:158:	log.G(ctx).WithFields(logrus.Fields{
internal/cmd/io.go:71:		log = log.WithFields(logrus.Fields{
internal/cmd/io_binary.go:110:	log.G(ctx).WithFields(logrus.Fields{
internal/cmd/io_npipe.go:26:	log.G(ctx).WithFields(logrus.Fields{
internal/cmd/io_npipe.go:111:				log.G(nprw.ctx).WithFields(logrus.Fields{
internal/computecore/computecore.go:137:		log.G(ctx).WithFields(logrus.Fields{
internal/devices/pnp.go:60:		log.G(ctx).WithFields(logrus.Fields{
internal/gcs-sidecar/bridge.go:141:		logrus.WithFields(logrus.Fields{
internal/gcs-sidecar/host.go:108:	logrus.WithFields(logrus.Fields{
internal/gcs/bridge.go:296:		brdg.log.WithFields(logrus.Fields{
internal/gcs/bridge.go:316:					brdg.log.WithFields(logrus.Fields{
internal/gcs/bridge.go:406:		brdg.log.WithFields(logrus.Fields{
internal/gcs/process.go:257:			log.G(ctx).WithFields(logrus.Fields{
internal/guest/bridge/bridge.go:83:		logrus.WithFields(logrus.Fields{
internal/guest/kmsg/kmsg.go:62:func (ke *Entry) logFormat() logrus.Fields {
internal/guest/kmsg/kmsg.go:63:	return logrus.Fields{
internal/guest/kmsg/kmsg.go:135:			logrus.WithFields(logrus.Fields{
internal/guest/network/netns.go:88:	ctx, entry := log.S(ctx, logrus.Fields{
internal/guest/network/netns.go:197:		log.G(ctx).WithFields(logrus.Fields{
internal/guest/network/netns.go:210:		log.G(ctx).WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/container.go:102:				entity.WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/container.go:183:	logrus.WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/container.go:217:	log.G(ctx).WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/process.go:113:				log.G(ctx).WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/process.go:244:		log.G(ctx).WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/uvm.go:161:		logrus.WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/uvm.go:335:		logrus.WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/uvm.go:365:		logrus.WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/uvm.go:569:		log.G(ctx).WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/uvm.go:1461:		logrus.WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/uvm.go:1486:	logrus.WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/uvm.go:1525:	logrus.WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/uvm.go:1563:	logrus.WithFields(logrus.Fields{
internal/guest/runtime/runc/container.go:84:	logrus.WithFields(logrus.Fields{
internal/guest/runtime/runc/container.go:248:	logrus.WithFields(logrus.Fields{
internal/guest/spec/spec.go:52:	logrus.WithFields(logrus.Fields{
internal/guest/spec/spec.go:475:			log.G(ctx).WithFields(logrus.Fields{
internal/guest/spec/spec_devices.go:71:				entry.WithFields(logrus.Fields{
internal/guest/stdio/stdio.go:171:		logrus.WithFields(logrus.Fields{
internal/guest/stdio/stdio.go:187:			logrus.WithFields(logrus.Fields{
internal/guest/stdio/stdio.go:193:		logrus.WithFields(logrus.Fields{
internal/guest/stdio/stdio.go:199:		logrus.WithFields(logrus.Fields{
internal/guest/stdio/stdio.go:213:				logrus.WithFields(logrus.Fields{
internal/guest/stdio/stdio.go:219:				logrus.WithFields(logrus.Fields{
internal/guest/stdio/stdio.go:289:					logrus.WithFields(logrus.Fields{
internal/guest/stdio/stdio.go:337:				logrus.WithFields(logrus.Fields{
internal/guest/stdio/stdio.go:348:				logrus.WithFields(logrus.Fields{
internal/guest/storage/crypt/crypt.go:51:			log.G(ctx).WithError(err).WithFields(logrus.Fields{
internal/guest/storage/overlay/overlay.go:49:	log.G(ctx).WithFields(logrus.Fields{
internal/guest/storage/scsi/scsi.go:176:		log.G(ctx).WithFields(logrus.Fields{
internal/guest/transport/devnull.go:23:	logrus.WithFields(logrus.Fields{
internal/guest/transport/vsock.go:26:	logrus.WithFields(logrus.Fields{
internal/hcs/callback.go:149:	log := logrus.WithFields(logrus.Fields{
internal/hcs/system.go:553:	logEntry.WithFields(logrus.Fields{
internal/hcsoci/create.go:131:	log.G(ctx).WithFields(logrus.Fields{
internal/hcsoci/create.go:209:		ctx, _ = log.SetEntry(ctx, logrus.Fields{logfields.UVMID: coi.HostingSystem.ID()})
internal/hcsoci/hcsdoc_wcow.go:222:		log.G(ctx).WithFields(logrus.Fields{
internal/hcsoci/hcsdoc_wcow.go:246:			log.G(ctx).WithFields(logrus.Fields{
internal/hcsoci/network.go:29:	log.G(ctx).WithFields(logrus.Fields{
internal/hcsoci/network.go:43:		log.G(ctx).WithFields(logrus.Fields{
internal/hcsoci/resources.go:25:		log.G(ctx).WithFields(logrus.Fields{
internal/hcsoci/resources.go:40:		log.G(ctx).WithFields(logrus.Fields{
internal/jobcontainers/mounts.go:104:			log.G(ctx).WithFields(logrus.Fields{
internal/jobobject/iocp.go:57:				log.G(ctx).WithFields(logrus.Fields{
internal/jobobject/iocp.go:69:				log.G(ctx).WithFields(logrus.Fields{
internal/layers/lcow.go:64:			log.G(ctx).WithFields(logrus.Fields{
internal/layers/lcow.go:198:			log.G(ctx).WithFields(logrus.Fields{
internal/layers/lcow.go:222:	log.G(ctx).WithFields(logrus.Fields{
internal/layers/wcow_mount.go:252:	log.G(ctx).WithFields(logrus.Fields{
internal/layers/wcow_mount.go:341:	log.G(ctx).WithFields(logrus.Fields{
internal/layers/wcow_mount.go:392:			log.G(ctx).WithFields(logrus.Fields{
internal/layers/wcow_mount.go:512:	log.G(ctx).WithFields(logrus.Fields{
internal/layers/wcow_mount.go:539:	log.G(ctx).WithFields(logrus.Fields{
internal/layers/wcow_mount.go:582:	log.G(ctx).WithFields(logrus.Fields{
internal/layers/wcow_mount.go:612:	log.G(ctx).WithFields(logrus.Fields{
internal/lcow/common.go:48:	log.G(ctx).WithFields(logrus.Fields{
internal/lcow/disk.go:29:	log.G(ctx).WithFields(logrus.Fields{
internal/lcow/disk.go:43:	log.G(ctx).WithFields(logrus.Fields{
internal/lcow/scratch.go:47:	log.G(ctx).WithFields(logrus.Fields{
internal/lcow/scratch.go:59:			log.G(ctx).WithFields(logrus.Fields{
internal/lcow/scratch.go:93:	log.G(ctx).WithFields(logrus.Fields{
internal/log/context.go:56:func SetEntry(ctx context.Context, fields logrus.Fields) (context.Context, *logrus.Entry) {
internal/log/format.go:69:		G(ctx).WithFields(logrus.Fields{
internal/oc/exporter.go:34:		logrus.WithFields(logrus.Fields{
internal/oc/exporter.go:49:	data := make(logrus.Fields, len(entry.Data)+len(s.Attributes)+10)
internal/oci/annotations.go:46:					log.G(ctx).WithFields(logrus.Fields{
internal/oci/annotations.go:69:		log.G(ctx).WithFields(logrus.Fields{
internal/oci/annotations.go:118:		entry := log.G(ctx).WithFields(logrus.Fields{
internal/oci/annotations.go:230:		entry := log.G(ctx).WithFields(logrus.Fields{
internal/oci/annotations.go:250:			entry.WithFields(logrus.Fields{
internal/oci/annotations.go:405:	entry := log.G(ctx).WithFields(logrus.Fields{
internal/oci/uvm.go:140:			log.G(ctx).WithFields(logrus.Fields{
internal/oci/uvm.go:294:			log.G(ctx).WithFields(logrus.Fields{
internal/tools/networkagent/main.go:470:	log.G(ctx).WithFields(logrus.Fields{
internal/tools/rootfs/main.go:70:				logrus.WithFields(logrus.Fields{
internal/tools/rootfs/merge.go:141:	logrus.WithFields(logrus.Fields{
internal/tools/rootfs/merge.go:178:	logrus.WithFields(logrus.Fields{
internal/tools/rootfs/merge.go:204:		logrus.WithFields(logrus.Fields{
internal/tools/rootfs/merge.go:259:		entry := logrus.WithFields(logrus.Fields{
internal/tools/rootfs/merge.go:321:			entry.WithFields(logrus.Fields{
internal/tools/rootfs/merge.go:336:			entry.WithFields(logrus.Fields{
internal/tools/uvmboot/lcow.go:300:			entry = entry.WithFields(logrus.Fields{
internal/tools/uvmboot/mounts.go:34:		log.G(ctx).WithFields(logrus.Fields{
internal/tools/uvmboot/mounts.go:62:		log.G(ctx).WithFields(logrus.Fields{
internal/uvm/cimfs.go:121:					log.G(ctx).WithFields(logrus.Fields{
internal/uvm/cimfs.go:140:		log.G(ctx).WithFields(logrus.Fields{
internal/uvm/computeagent.go:66:	log.G(ctx).WithFields(logrus.Fields{
internal/uvm/computeagent.go:84:	log.G(ctx).WithFields(logrus.Fields{
internal/uvm/computeagent.go:100:	log.G(ctx).WithFields(logrus.Fields{
internal/uvm/computeagent.go:166:	log.G(ctx).WithFields(logrus.Fields{
internal/uvm/computeagent.go:216:	log.G(ctx).WithFields(logrus.Fields{
internal/uvm/create.go:282:	log.G(ctx).WithFields(logrus.Fields{
internal/uvm/create_lcow.go:159:		log.G(ctx).WithFields(logrus.Fields{
internal/uvm/create_lcow.go:176:		log.G(ctx).WithFields(logrus.Fields{
internal/uvm/create_lcow.go:190:			log.G(ctx).WithFields(logrus.Fields{
internal/uvm/create_lcow.go:943:			log.G(ctx).WithFields(logrus.Fields{
internal/uvm/create_lcow.go:951:			log.G(ctx).WithFields(logrus.Fields{
internal/uvm/create_wcow.go:127:	log.G(ctx).WithFields(logrus.Fields{
internal/uvm/create_wcow.go:600:			log.G(ctx).WithFields(logrus.Fields{
internal/uvm/create_wcow.go:608:			log.G(ctx).WithFields(logrus.Fields{
internal/uvm/network.go:313:			log.G(ctx).WithFields(logrus.Fields{
internal/uvm/stats.go:60:		log.G(ctx).WithFields(logrus.Fields{
internal/uvm/vpmem.go:64:			log.G(ctx).WithFields(logrus.Fields{
internal/uvm/vpmem.go:80:			log.G(ctx).WithFields(logrus.Fields{
internal/uvm/vpmem.go:173:	log.G(ctx).WithFields(logrus.Fields{
internal/uvm/vpmem_mapped.go:123:	log.G(ctx).WithFields(logrus.Fields{
internal/uvm/vpmem_mapped.go:138:	log.G(ctx).WithFields(logrus.Fields{
internal/uvm/vpmem_mapped.go:164:	log.G(ctx).WithFields(logrus.Fields{
internal/uvm/vpmem_mapped.go:177:				log.G(ctx).WithFields(logrus.Fields{
internal/uvm/vpmem_mapped.go:213:		log.G(ctx).WithFields(logrus.Fields{
internal/uvm/vsmb.go:216:		log.G(ctx).WithFields(logrus.Fields{
internal/uvm/vsmb.go:301:		log.G(ctx).WithFields(logrus.Fields{
internal/uvmfolder/locate.go:35:	log.G(ctx).WithFields(logrus.Fields{
internal/verity/verity.go:41:	log.G(ctx).WithFields(logrus.Fields{
internal/vhdx/info.go:169:			log.G(ctx).WithFields(logrus.Fields{
internal/vhdx/info.go:183:			log.G(ctx).WithFields(logrus.Fields{
internal/vhdx/info.go:226:	log.G(ctx).WithFields(logrus.Fields{
internal/vm/vmmanager/uvm.go:59:	log.G(ctx).WithFields(logrus.Fields{
internal/vm/vmutils/gcs_logs.go:68:					logrus.WithFields(logrus.Fields{
internal/vm/vmutils/gcs_logs.go:79:					logrus.WithFields(logrus.Fields{
internal/vm/vmutils/normalize.go:28:		log.G(ctx).WithFields(logrus.Fields{
internal/vm/vmutils/normalize.go:49:		log.G(ctx).WithFields(logrus.Fields{
internal/vm/vmutils/numa.go:65:			log.G(ctx).WithFields(logrus.Fields{
internal/vm/vmutils/numa.go:72:			log.G(ctx).WithFields(logrus.Fields{
internal/vm/vmutils/numa.go:107:			entry.WithFields(logrus.Fields{
internal/vmcompute/vmcompute.go:95:		log.G(ctx).WithFields(logrus.Fields{
internal/wclayer/cim/mount.go:69:	log.G(ctx).WithFields(logrus.Fields{
internal/wclayer/cim/mount.go:128:	log.L.WithFields(logrus.Fields{
internal/windevice/devicequery.go:238:	log.G(ctx).WithFields(logrus.Fields{
internal/windevice/devicequery.go:293:		log.G(ctx).WithFields(logrus.Fields{
pkg/cimfs/cim_writer_windows.go:378:	log.G(ctx).WithFields(logrus.Fields{
pkg/ociwclayer/cim/import.go:38:	log.G(ctx).WithFields(logrus.Fields{
pkg/ociwclayer/cim/import.go:303:	log.G(ctx).WithFields(logrus.Fields{
pkg/securitypolicy/securitypolicy_options.go:139:	log.G(ctx).WithFields(logrus.Fields{
pkg/securitypolicy/securitypolicy_options.go:146:	log.G(ctx).WithFields(logrus.Fields{
test/functional/main_test.go:220:					log.G(ctx).WithFields(logrus.Fields{
test/functional/main_test.go:368:	e := log.G(ctx).WithFields(logrus.Fields{
test/functional/main_test.go:377:		e.WithFields(logrus.Fields{
test/internal/layers/lazy.go:62:	log.G(ctx).WithFields(logrus.Fields{
test/internal/layers/lazy.go:124:	log.G(ctx).WithFields(logrus.Fields{
test/internal/layers/lazy.go:288:		log.L.WithFields(logrus.Fields{
vendor/github.com/open-policy-agent/opa/logging/logging.go:225:// Fields adapts the RequestContext fields to logrus.Fields.
vendor/github.com/open-policy-agent/opa/logging/logging.go:226:func (rctx RequestContext) Fields() logrus.Fields {
vendor/github.com/open-policy-agent/opa/logging/logging.go:227:	return logrus.Fields{
vendor/github.com/sirupsen/logrus/README.md:212:  log.WithFields(logrus.Fields{
```

## logrus.Entry usages

```txt
internal/cmd/cmd.go:50:	Log *logrus.Entry
internal/cmd/io.go:67:func relayIO(w io.Writer, r io.Reader, log *logrus.Entry, name string) (int64, error) {
internal/gcs/bridge.go:58:	log     *logrus.Entry
internal/gcs/bridge.go:75:func newBridge(conn io.ReadWriteCloser, notify notifyFunc, log *logrus.Entry) *bridge {
internal/gcs/guestconnection.go:62:	Log *logrus.Entry
internal/guest/transport/log.go:13:	entry *logrus.Entry
internal/log/context.go:32:// GetEntry returns a `logrus.Entry` stored in the context, if one exists.
internal/log/context.go:39:func GetEntry(ctx context.Context) *logrus.Entry {
internal/log/context.go:56:func SetEntry(ctx context.Context, fields logrus.Fields) (context.Context, *logrus.Entry) {
internal/log/context.go:86:func WithContext(ctx context.Context, entry *logrus.Entry) (context.Context, *logrus.Entry) {
internal/log/context.go:95:func fromContext(ctx context.Context) *logrus.Entry {
internal/log/context.go:96:	e, _ := ctx.Value(_entryContextKey).(*logrus.Entry)
internal/log/hook.go:15:// Hook intercepts and formats a [logrus.Entry] before it logged.
internal/log/hook.go:43:	// the entry from the span context stored in [logrus.Entry.Context], if it exists.
internal/log/hook.go:61:func (h *Hook) Fire(e *logrus.Entry) (err error) {
internal/log/hook.go:69:// encode loops through all the fields in the [logrus.Entry] and encodes them according to
internal/log/hook.go:80:func (h *Hook) encode(e *logrus.Entry) {
internal/log/hook.go:161:func (h *Hook) addSpanContext(e *logrus.Entry) {
internal/log/nopformatter.go:12:func (NopFormatter) Format(*logrus.Entry) ([]byte, error) { return nil, nil }
internal/vm/guestmanager/guest.go:21:	log *logrus.Entry
internal/vm/vmutils/gcs_logs_test.go:203:		validate      func(t *testing.T, entries []*logrus.Entry)
internal/vm/vmutils/gcs_logs_test.go:210:			validate: func(t *testing.T, entries []*logrus.Entry) {
internal/vm/vmutils/gcs_logs_test.go:225:			validate: func(t *testing.T, entries []*logrus.Entry) {
internal/vm/vmutils/gcs_logs_test.go:246:			validate: func(t *testing.T, entries []*logrus.Entry) {
internal/vm/vmutils/gcs_logs_test.go:394:	Entries []*logrus.Entry
internal/vm/vmutils/gcs_logs_test.go:401:func (h *testLogHook) Fire(entry *logrus.Entry) error {
vendor/github.com/Microsoft/go-winio/pkg/etwlogrus/hook.go:27:	getName func(*logrus.Entry) string
vendor/github.com/Microsoft/go-winio/pkg/etwlogrus/hook.go:29:	getEventsOpts func(*logrus.Entry) []etw.EventOpt
vendor/github.com/Microsoft/go-winio/pkg/etwlogrus/hook.go:90:func (h *Hook) Fire(e *logrus.Entry) error {
vendor/github.com/Microsoft/go-winio/pkg/etwlogrus/opts.go:40:func WithGetName(f func(*logrus.Entry) string) HookOpt {
vendor/github.com/Microsoft/go-winio/pkg/etwlogrus/opts.go:48:func WithEventOpts(f func(*logrus.Entry) []etw.EventOpt) HookOpt {
vendor/github.com/containerd/log/context.go:70:// Entry is a transitional type, and currently an alias for [logrus.Entry].
vendor/github.com/containerd/log/context.go:71:type Entry = logrus.Entry
vendor/github.com/sirupsen/logrus/README.md:174:  // the logrus.Entry returned from WithFields()
vendor/github.com/sirupsen/logrus/README.md:249:every line, you can create a `logrus.Entry` to pass around instead:
```

## SetFormatter usages

```txt
cmd/containerd-shim-lcow-v2/main.go:35:	logrus.SetFormatter(log.NopFormatter{})
cmd/containerd-shim-runhcs-v1/serve.go:111:			logrus.SetFormatter(&logrus.TextFormatter{
cmd/containerd-shim-runhcs-v1/serve.go:158:			logrus.SetFormatter(hcslog.NopFormatter{})
cmd/gcs/main.go:261:		logrus.SetFormatter(&logrus.JSONFormatter{
cmd/runhcs/main.go:154:			logrus.SetFormatter(new(logrus.JSONFormatter))
internal/tools/rootfs/main.go:25:	logrus.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
test/functional/main_test.go:175:	logrus.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
test/functional/main_test.go:250:			logrus.SetFormatter(log.NopFormatter{})
test/functional/main_test.go:255:				logrus.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
test/gcs/main_test.go:118:	logrus.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
vendor/github.com/containerd/log/context.go:154:		L.Logger.SetFormatter(&logrus.TextFormatter{
vendor/github.com/containerd/log/context.go:160:		L.Logger.SetFormatter(&logrus.JSONFormatter{
vendor/github.com/open-policy-agent/opa/logging/logging.go:65:// SetFormatter sets the underlying logrus formatter.
vendor/github.com/open-policy-agent/opa/logging/logging.go:66:func (l *StandardLogger) SetFormatter(formatter logrus.Formatter) {
vendor/github.com/open-policy-agent/opa/logging/logging.go:67:	l.logger.SetFormatter(formatter)
vendor/github.com/opencontainers/runc/libcontainer/init_linux.go:179:	logrus.SetFormatter(new(logrus.JSONFormatter))
vendor/github.com/sirupsen/logrus/CHANGELOG.md:116:    * SetFormatter
vendor/github.com/sirupsen/logrus/README.md:43:With `log.SetFormatter(&log.JSONFormatter{})`, for easy parsing by logstash
vendor/github.com/sirupsen/logrus/README.md:63:With the default `log.SetFormatter(&log.TextFormatter{})` when a TTY is not
vendor/github.com/sirupsen/logrus/README.md:78:	log.SetFormatter(&log.TextFormatter{
vendor/github.com/sirupsen/logrus/README.md:147:  log.SetFormatter(&log.JSONFormatter{})
vendor/github.com/sirupsen/logrus/README.md:350:    log.SetFormatter(&log.JSONFormatter{})
vendor/github.com/sirupsen/logrus/README.md:353:    log.SetFormatter(&log.TextFormatter{})
vendor/github.com/sirupsen/logrus/README.md:399:log.SetFormatter(new(MyJSONFormatter))
vendor/github.com/sirupsen/logrus/exported.go:23:// SetFormatter sets the standard logger formatter.
vendor/github.com/sirupsen/logrus/exported.go:24:func SetFormatter(formatter Formatter) {
vendor/github.com/sirupsen/logrus/exported.go:25:	std.SetFormatter(formatter)
vendor/github.com/sirupsen/logrus/logger.go:383:// SetFormatter sets the logger formatter.
vendor/github.com/sirupsen/logrus/logger.go:384:func (logger *Logger) SetFormatter(formatter Formatter) {
```

## SetOutput usages

```txt
cmd/containerd-shim-lcow-v2/main.go:36:	logrus.SetOutput(io.Discard)
cmd/containerd-shim-lcow-v2/manager.go:121:	logrus.SetOutput(io.Discard)
cmd/containerd-shim-runhcs-v1/serve.go:150:					logrus.SetOutput(cur)
cmd/containerd-shim-runhcs-v1/serve.go:159:			logrus.SetOutput(io.Discard)
cmd/containerd-shim-runhcs-v1/start.go:40:		logrus.SetOutput(io.Discard)
cmd/gcs/main.go:247:		logrus.SetOutput(logWriter)
cmd/gcs/main.go:250:		logrus.SetOutput(io.Discard)
cmd/gcstools/commoncli/common.go:38:	logrus.SetOutput(outputTarget)
cmd/gcstools/installdrivers.go:90:	log.G(ctx).Logger.SetOutput(os.Stderr)
cmd/ncproxy/run.go:202:		logrus.SetOutput(io.Discard)
cmd/ncproxy/service.go:90:	log.SetOutput(os.Stderr)
cmd/runhcs/main.go:148:			logrus.SetOutput(f)
cmd/runhcs/shim.go:52:			logrus.SetOutput(lpc)
cmd/runhcs/shim.go:54:			logrus.SetOutput(os.Stderr)
cmd/runhcs/vm.go:45:			logrus.SetOutput(lpc)
cmd/runhcs/vm.go:47:			logrus.SetOutput(os.Stderr)
internal/guest/bridge/bridge_unit_test.go:460:	logrus.SetOutput(io.Discard)
internal/guest/bridge/bridge_unit_test.go:514:	logrus.SetOutput(io.Discard)
internal/guest/bridge/bridge_unit_test.go:595:	logrus.SetOutput(io.Discard)
internal/shim/shim.go:187:	l.Logger.SetOutput(f)
internal/vm/vmutils/gcs_logs_test.go:190:	logrus.SetOutput(io.Discard)
internal/vm/vmutils/gcs_logs_test.go:193:		logrus.SetOutput(originalOutput)
internal/vm/vmutils/gcs_logs_test.go:277:	logrus.SetOutput(io.Discard)
internal/vm/vmutils/gcs_logs_test.go:280:		logrus.SetOutput(originalOutput)
internal/vm/vmutils/gcs_logs_test.go:299:	logrus.SetOutput(io.Discard)
internal/vm/vmutils/gcs_logs_test.go:302:		logrus.SetOutput(originalOutput)
internal/vm/vmutils/gcs_logs_test.go:351:	logrus.SetOutput(io.Discard)
internal/vm/vmutils/gcs_logs_test.go:354:		logrus.SetOutput(originalOutput)
internal/vm/vmutils/gcs_logs_test.go:424:	logrus.SetOutput(io.Discard)
pkg/securitypolicy/securitypolicy_options.go:90:		logrus.SetOutput(s.logWriter)
pkg/securitypolicy/securitypolicy_options.go:92:		logrus.SetOutput(io.Discard)
test/functional/main_test.go:174:	logrus.SetOutput(os.Stdout)
test/functional/main_test.go:251:			logrus.SetOutput(io.Discard)
test/functional/main_test.go:256:				logrus.SetOutput(os.Stdout)
test/gcs/main_test.go:117:	logrus.SetOutput(os.Stdout)
test/internal/schemaversion_test.go:18:	logrus.SetOutput(io.Discard)
vendor/github.com/containerd/containerd/v2/pkg/shim/shim.go:185:	l.Logger.SetOutput(f)
vendor/github.com/open-policy-agent/opa/logging/logging.go:60:// SetOutput sets the underlying logrus output.
vendor/github.com/open-policy-agent/opa/logging/logging.go:61:func (l *StandardLogger) SetOutput(w io.Writer) {
vendor/github.com/open-policy-agent/opa/logging/logging.go:62:	l.logger.SetOutput(w)
vendor/github.com/opencontainers/runc/libcontainer/init_linux.go:178:	logrus.SetOutput(logPipe)
vendor/github.com/sirupsen/logrus/CHANGELOG.md:117:    * SetOutput
vendor/github.com/sirupsen/logrus/CHANGELOG.md:133:  * a new SetOutput method in the Logger
vendor/github.com/sirupsen/logrus/README.md:151:  log.SetOutput(os.Stdout)
vendor/github.com/sirupsen/logrus/README.md:440:log.SetOutput(logger.Writer())
vendor/github.com/sirupsen/logrus/exported.go:18:// SetOutput sets the standard logger output.
vendor/github.com/sirupsen/logrus/exported.go:19:func SetOutput(out io.Writer) {
vendor/github.com/sirupsen/logrus/exported.go:20:	std.SetOutput(out)
vendor/github.com/sirupsen/logrus/logger.go:390:// SetOutput sets the logger output.
vendor/github.com/sirupsen/logrus/logger.go:391:func (logger *Logger) SetOutput(output io.Writer) {
vendor/github.com/urfave/cli/flag.go:122:	set.SetOutput(ioutil.Discard)
vendor/github.com/urfave/cli/v2/flag.go:181:	set.SetOutput(io.Discard)
vendor/golang.org/x/tools/internal/stdlib/manifest.go:5031:		{"(*FlagSet).SetOutput", Method, 0, ""},
vendor/golang.org/x/tools/internal/stdlib/manifest.go:7232:		{"(*Logger).SetOutput", Method, 5, ""},
vendor/golang.org/x/tools/internal/stdlib/manifest.go:7259:		{"SetOutput", Func, 0, "func(w io.Writer)"},
```

## SetLevel usages

```txt
cmd/containerd-shim-lcow-v2/main.go:83:				logrus.SetLevel(lvl)
cmd/containerd-shim-runhcs-v1/serve.go:96:			logrus.SetLevel(logrus.DebugLevel)
cmd/containerd-shim-runhcs-v1/serve.go:106:			logrus.SetLevel(lvl)
cmd/gcs-sidecar/main.go:155:	logrus.SetLevel(level)
cmd/gcs/main.go:275:	logrus.SetLevel(level)
cmd/gcstools/commoncli/common.go:30:	logrus.SetLevel(level)
cmd/runhcs/main.go:135:			logrus.SetLevel(logrus.DebugLevel)
internal/oci/annotations_test.go:25:		logrus.SetLevel(l)
internal/oci/annotations_test.go:27:	logrus.SetLevel(logrus.ErrorLevel)
internal/oci/annotations_test.go:144:		logrus.SetLevel(l)
internal/oci/annotations_test.go:146:	logrus.SetLevel(logrus.ErrorLevel)
internal/shim/shim.go:181:		_ = log.SetLevel("debug")
internal/shim/shim.go:266:			logger.Logger.SetLevel(log.DebugLevel)
internal/tools/rootfs/main.go:57:						logrus.SetLevel(lvl)
internal/tools/uvmboot/main.go:108:		logrus.SetLevel(lvl)
test/functional/main_test.go:176:	logrus.SetLevel(flagLogLevel.Level)
test/gcs/main_test.go:115:	logrus.SetLevel(flagLogLevel.Level)
vendor/github.com/containerd/containerd/v2/pkg/shim/shim.go:179:		_ = log.SetLevel("debug")
vendor/github.com/containerd/containerd/v2/pkg/shim/shim.go:264:			logger.Logger.SetLevel(log.DebugLevel)
vendor/github.com/containerd/log/context.go:111:// SetLevel sets log level globally. It returns an error if the given
vendor/github.com/containerd/log/context.go:123:func SetLevel(level string) error {
vendor/github.com/containerd/log/context.go:129:	L.Logger.SetLevel(lvl)
vendor/github.com/open-policy-agent/opa/logging/logging.go:35:	SetLevel(Level)
vendor/github.com/open-policy-agent/opa/logging/logging.go:88:// SetLevel sets the standard logger level.
vendor/github.com/open-policy-agent/opa/logging/logging.go:89:func (l *StandardLogger) SetLevel(level Level) {
vendor/github.com/open-policy-agent/opa/logging/logging.go:105:	l.logger.SetLevel(logrusLevel)
vendor/github.com/open-policy-agent/opa/logging/logging.go:197:// SetLevel set log level
vendor/github.com/open-policy-agent/opa/logging/logging.go:198:func (l *NoOpLogger) SetLevel(level Level) {
vendor/github.com/opencontainers/runc/libcontainer/init_linux.go:169:		logrus.SetLevel(logrus.Level(logLevel))
vendor/github.com/sirupsen/logrus/CHANGELOG.md:154:* Make (*Logger) SetLevel a public method
vendor/github.com/sirupsen/logrus/README.md:154:  log.SetLevel(log.WarnLevel)
vendor/github.com/sirupsen/logrus/README.md:314:log.SetLevel(log.InfoLevel)
vendor/github.com/sirupsen/logrus/README.md:320:Note: If you want different log levels for global (`log.SetLevel(...)`) and syslog logging, please check the [syslog hook README](hooks/syslog/README.md#different-log-levels-for-local-and-remote-logging).
vendor/github.com/sirupsen/logrus/exported.go:34:// SetLevel sets the standard logger level.
vendor/github.com/sirupsen/logrus/exported.go:35:func SetLevel(level Level) {
vendor/github.com/sirupsen/logrus/exported.go:36:	std.SetLevel(level)
vendor/github.com/sirupsen/logrus/logger.go:361:// SetLevel sets the logger level.
vendor/github.com/sirupsen/logrus/logger.go:362:func (logger *Logger) SetLevel(level Level) {
vendor/go.opentelemetry.io/otel/semconv/v1.39.0/attribute_group.go:10346:	McpMethodNameLoggingSetLevel = McpMethodNameKey.String("logging/setLevel")
```

## AddHook usages

```txt
cmd/containerd-shim-lcow-v2/main.go:29:	logrus.AddHook(log.NewHook())
cmd/containerd-shim-lcow-v2/service/plugin/plugin.go:40:			logrus.AddHook(hook)
cmd/containerd-shim-runhcs-v1/main.go:72:	logrus.AddHook(log.NewHook())
cmd/containerd-shim-runhcs-v1/main.go:81:			logrus.AddHook(hook)
cmd/gcs-sidecar/main.go:149:	logrus.AddHook(shimlog.NewHook())
cmd/gcs/main.go:225:	logrus.AddHook(log.NewHook())
cmd/ncproxy/run.go:153:			logrus.AddHook(hook)
cmd/runhcs/main.go:60:		logrus.AddHook(hook)
internal/vm/vmutils/gcs_logs_test.go:191:	logrus.AddHook(hook)
internal/vm/vmutils/gcs_logs_test.go:278:	logrus.AddHook(hook)
internal/vm/vmutils/gcs_logs_test.go:300:	logrus.AddHook(hook)
internal/vm/vmutils/gcs_logs_test.go:352:	logrus.AddHook(hook)
test/functional/main_test.go:243:					logrus.AddHook(hook)
vendor/github.com/sirupsen/logrus/README.md:278:  log.AddHook(airbrake.NewHook(123, "xyz", "production"))
vendor/github.com/sirupsen/logrus/README.md:284:    log.AddHook(hook)
vendor/github.com/sirupsen/logrus/exported.go:49:// AddHook adds a hook to the standard logger hooks.
vendor/github.com/sirupsen/logrus/exported.go:50:func AddHook(hook Hook) {
vendor/github.com/sirupsen/logrus/exported.go:51:	std.AddHook(hook)
vendor/github.com/sirupsen/logrus/logger.go:371:// AddHook adds a hook to the logger hooks.
vendor/github.com/sirupsen/logrus/logger.go:372:func (logger *Logger) AddHook(hook Hook) {
```

## Context logger usages

```txt
cmd/containerd-shim-lcow-v2/service/service.go:116:			log.G(ctx).WithError(err).Error("post event")
cmd/containerd-shim-lcow-v2/service/service_sandbox_internal.go:276:			log.G(ctx).WithError(err).Error("failed to terminate VM during shutdown")
cmd/containerd-shim-runhcs-v1/exec_hcs.go:46:	log.G(ctx).WithFields(logrus.Fields{
cmd/containerd-shim-runhcs-v1/exec_hcs.go:205:		Log: log.G(ctx).WithFields(logrus.Fields{
cmd/containerd-shim-runhcs-v1/exec_hcs.go:309:							log.G(ctx).WithField("err", deliveryErr).Errorf("Error in delivering signal %d, to pid: %d", signal, he.pid)
cmd/containerd-shim-runhcs-v1/exec_hcs.go:313:						log.G(ctx).Errorf("Error: NotFound; exec: '%s' in task: '%s' not found", he.id, he.tid)
cmd/containerd-shim-runhcs-v1/exec_hcs.go:409:		log.G(ctx).WithField("status", status).Debug("hcsExec::exitFromCreatedL")
cmd/containerd-shim-runhcs-v1/exec_hcs.go:460:		log.G(ctx).WithError(err).Error("failed process Wait")
cmd/containerd-shim-runhcs-v1/exec_hcs.go:469:		log.G(ctx).WithError(err).Error("failed to get ExitCode")
cmd/containerd-shim-runhcs-v1/exec_hcs.go:471:		log.G(ctx).WithField("exitCode", code).Debug("exited")
cmd/containerd-shim-runhcs-v1/exec_hcs.go:498:			log.G(ctx).WithError(err).Error("failed to publish TaskExitEvent")
cmd/containerd-shim-runhcs-v1/exec_wcow_podsandbox.go:22:	log.G(ctx).WithFields(logrus.Fields{
cmd/containerd-shim-runhcs-v1/exec_wcow_podsandbox.go:197:		log.G(ctx).WithField("status", status).Debug("wcowPodSandboxExec::ForceExit")
cmd/containerd-shim-runhcs-v1/pod.go:40:	log.G(ctx).WithField("options", log.Format(ctx, *wopts)).Debug("initialize WCOW boot files")
cmd/containerd-shim-runhcs-v1/pod.go:136:	log.G(ctx).WithField("tid", req.ID).Debug("createPod")
cmd/containerd-shim-runhcs-v1/pod.go:306:				log.G(ctx).Warning("Detected CMD override for pause container entrypoint." +
cmd/containerd-shim-runhcs-v1/service_internal.go:89:		if entry := log.G(ctx); entry.Logger.IsLevelEnabled(logrus.DebugLevel) {
cmd/containerd-shim-runhcs-v1/task_hcs.go:56:	log.G(ctx).WithField("tid", req.ID).Debug("newHcsStandaloneTask")
cmd/containerd-shim-runhcs-v1/task_hcs.go:191:	log.G(ctx).WithFields(logrus.Fields{
cmd/containerd-shim-runhcs-v1/task_hcs.go:436:				log.G(ctx).WithFields(logrus.Fields{
cmd/containerd-shim-runhcs-v1/task_hcs.go:511:			log.G(ctx).Error("timed out waiting for resource cleanup")
cmd/containerd-shim-runhcs-v1/task_hcs.go:533:				log.G(ctx).WithError(err).Errorf("failed to delete container state")
cmd/containerd-shim-runhcs-v1/task_hcs.go:634:		log.G(ctx).WithError(err).Error("failed to wait for host virtual machine exit")
cmd/containerd-shim-runhcs-v1/task_hcs.go:636:		log.G(ctx).Debug("host virtual machine exited")
cmd/containerd-shim-runhcs-v1/task_hcs.go:658:		log.G(ctx).Debug("hcsTask::closeOnce")
cmd/containerd-shim-runhcs-v1/task_hcs.go:675:				log.G(ctx).WithError(err).Error("failed to shutdown container")
cmd/containerd-shim-runhcs-v1/task_hcs.go:683:						log.G(ctx).WithError(err).Error("failed to wait for container shutdown")
cmd/containerd-shim-runhcs-v1/task_hcs.go:687:					log.G(ctx).WithError(err).Error("failed to wait for container shutdown")
cmd/containerd-shim-runhcs-v1/task_hcs.go:694:					log.G(ctx).WithError(err).Error("failed to terminate container")
cmd/containerd-shim-runhcs-v1/task_hcs.go:702:							log.G(ctx).WithError(err).Error("failed to wait for container terminate")
cmd/containerd-shim-runhcs-v1/task_hcs.go:705:						log.G(ctx).WithError(hcs.ErrTimeout).Error("failed to wait for container terminate")
cmd/containerd-shim-runhcs-v1/task_hcs.go:712:				log.G(ctx).WithError(err).Error("failed to release container resources")
cmd/containerd-shim-runhcs-v1/task_hcs.go:717:				log.G(ctx).WithError(err).Error("failed to close container")
cmd/containerd-shim-runhcs-v1/task_hcs.go:733:		log.G(ctx).Debug("hcsTask::closeHostOnce")
cmd/containerd-shim-runhcs-v1/task_hcs.go:737:				log.G(ctx).WithError(err).Error("failed host vm shutdown")
cmd/containerd-shim-runhcs-v1/task_hcs.go:753:			log.G(ctx).WithError(err).Error("failed to publish TaskExitEventTopic")
cmd/containerd-shim-runhcs-v1/task_hcs.go:781:			log.G(ctx).WithError(err).Warn("failed to capture guest stacks")
cmd/containerd-shim-runhcs-v1/task_wcow_podsandbox.go:39:	log.G(ctx).WithField("tid", id).Debug("newWcowPodSandboxTask")
cmd/containerd-shim-runhcs-v1/task_wcow_podsandbox.go:187:		log.G(ctx).Debug("wcowPodSandboxTask::closeOnce")
cmd/containerd-shim-runhcs-v1/task_wcow_podsandbox.go:191:				log.G(ctx).WithError(err).Error("failed to cleanup networking for utility VM")
cmd/containerd-shim-runhcs-v1/task_wcow_podsandbox.go:195:				log.G(ctx).WithError(err).Error("failed host vm shutdown")
cmd/containerd-shim-runhcs-v1/task_wcow_podsandbox.go:211:			log.G(ctx).WithError(err).Error("failed to publish TaskExitEventTopic")
cmd/containerd-shim-runhcs-v1/task_wcow_podsandbox.go:236:		log.G(ctx).WithError(werr).Error("parent wait failed")
cmd/containerd-shim-runhcs-v1/task_wcow_podsandbox.go:268:			log.G(ctx).WithError(err).Warn("failed to capture guest stacks")
cmd/gcstools/installdrivers.go:90:	log.G(ctx).Logger.SetOutput(os.Stderr)
cmd/gcstools/installdrivers.go:92:		log.G(ctx).Fatalf("error while installing drivers: %s", err)
cmd/ncproxy/hcn.go:309:		log.G(ctx).WithField("networkName", network.Name).Warn("network has multiple MAC pools, only returning the first")
cmd/ncproxy/ncproxy.go:108:			log.G(ctx).WithError(err).Warn("Failed to query ncproxy networking database")
cmd/ncproxy/ncproxy.go:125:			log.G(ctx).WithField("iov settings", settings.Policies.IovPolicySettings).Info("AddNIC iov settings")
cmd/ncproxy/ncproxy.go:201:	log.G(ctx).WithField("iov settings", settings.Policies.IovPolicySettings).Info("ModifyNIC iov settings")
cmd/ncproxy/ncproxy.go:279:			log.G(ctx).WithError(err).Warn("Failed to query ncproxy networking database")
cmd/ncproxy/ncproxy.go:459:			log.G(ctx).WithError(err).Warn("Failed to query ncproxy networking database")
cmd/ncproxy/ncproxy.go:470:				log.G(ctx).WithField("namespaceID", req.NamespaceID).
cmd/ncproxy/ncproxy.go:480:			log.G(ctx).WithField("namespaceID", req.NamespaceID).Debug("Attaching endpoint to default host namespace")
cmd/ncproxy/ncproxy.go:511:			log.G(ctx).WithError(err).Warn("Failed to query ncproxy networking database")
cmd/ncproxy/ncproxy.go:546:			log.G(ctx).WithError(err).Warn("Failed to query ncproxy networking database")
cmd/ncproxy/ncproxy.go:612:		log.G(ctx).WithError(err).Warn("Failed to query ncproxy networking database")
cmd/ncproxy/ncproxy.go:691:		log.G(ctx).WithError(err).Warn("Failed to query ncproxy networking database")
cmd/ncproxy/ncproxy.go:812:		log.G(ctx).WithField("key", req.ContainerID).WithError(err).Warn("failed to delete key from compute agent store")
cmd/ncproxy/run.go:52:			log.G(ctx).Info("falling back to v0 nodenetsvc api")
cmd/ncproxy/run.go:224:		log.G(ctx).Infof("Connecting to NodeNetworkService at address %s", conf.NodeNetSvcAddr)
cmd/ncproxy/run.go:246:		log.G(ctx).Infof("Successfully connected to NodeNetworkService at address %s", conf.NodeNetSvcAddr)
cmd/ncproxy/run.go:277:	log.G(ctx).WithFields(logrus.Fields{
cmd/ncproxy/run.go:306:		log.G(ctx).Info("Received interrupt. Closing")
cmd/ncproxy/run.go:312:		log.G(ctx).Info("Windows service stopped or shutdown")
cmd/ncproxy/server.go:53:		log.G(ctx).WithError(err).Error("failed to create ttrpc server")
cmd/ncproxy/server.go:73:	log.G(ctx).Warnf("ncproxygprc api v0 is deprecated, please use ncproxygrpc api v1")
cmd/ncproxy/server.go:80:		log.G(ctx).WithError(err).Errorf("failed to listen on %s", s.conf.TTRPCAddr)
cmd/ncproxy/server.go:86:		log.G(ctx).WithError(err).Errorf("failed to listen on %s", s.conf.GRPCAddr)
cmd/ncproxy/server.go:96:		log.G(ctx).WithError(err).Error("failed to gracefully shutdown ttrpc server")
cmd/ncproxy/server.go:103:		log.G(ctx).WithError(err).Error("failed to disconnect connections in compute agent cache")
cmd/ncproxy/server.go:106:		log.G(ctx).WithError(err).Error("failed to close ncproxy compute agent database")
cmd/ncproxy/server.go:109:		log.G(ctx).WithError(err).Error("failed to close ncproxy networking database")
cmd/ncproxy/server.go:122:		log.G(ctx).WithFields(logrus.Fields{
cmd/ncproxy/server.go:131:		log.G(ctx).WithFields(logrus.Fields{
cmd/ncproxy/server.go:170:		log.G(ctx).WithError(err).Debug("no entries in database")
cmd/ncproxy/server.go:173:		log.G(ctx).WithError(err).Error("failed to get compute agent information")
cmd/ncproxy/server.go:184:				log.G(ctx).WithField("agentAddress", agentAddress).WithError(err).Error("failed to create new compute agent client")
cmd/ncproxy/server.go:187:					log.G(ctx).WithField("key", containerID).WithError(dErr).Warn("failed to delete key from compute agent store")
cmd/ncproxy/server.go:191:			log.G(ctx).WithField("containerID", containerID).Info("reconnected to container's compute agent")
cmd/ncproxy/server.go:212:			log.G(ctx).WithError(err).Error("failed to close compute agent connection")
internal/builder/vm/lcow/boot.go:25:	log.G(ctx).Debug("resolveBootFilesPath: starting boot files path resolution")
internal/builder/vm/lcow/boot.go:38:		log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/boot.go:48:	log.G(ctx).WithField(logfields.Path, bootFilesRootPath).Debug("resolveBootFilesPath completed successfully")
internal/builder/vm/lcow/boot.go:55:	log.G(ctx).Debug("parseBootOptions: starting boot options parsing")
internal/builder/vm/lcow/boot.go:63:	log.G(ctx).WithField(logfields.Path, bootFilesPath).Debug("using boot files path")
internal/builder/vm/lcow/boot.go:80:		log.G(ctx).WithField(
internal/builder/vm/lcow/boot.go:92:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/boot.go:108:			log.G(ctx).WithField(vmutils.UncompressedKernelFile, filepath.Join(bootFilesPath, vmutils.UncompressedKernelFile)).Debug("updated LCOW kernel file to " + vmutils.UncompressedKernelFile)
internal/builder/vm/lcow/boot.go:121:	log.G(ctx).WithField("kernelFile", kernelFileName).Debug("selected kernel file")
internal/builder/vm/lcow/boot.go:125:		log.G(ctx).WithField("preferredRootFSType", preferredRootfsType).Debug("applying preferred rootfs type override")
internal/builder/vm/lcow/boot.go:139:	log.G(ctx).WithField("rootFsFile", rootFsFile).Debug("selected rootfs file")
internal/builder/vm/lcow/boot.go:143:		log.G(ctx).Debug("configuring kernel direct boot")
internal/builder/vm/lcow/boot.go:150:			log.G(ctx).WithField("initrdPath", chipset.LinuxKernelDirect.InitRdPath).Debug("configured initrd for kernel direct boot")
internal/builder/vm/lcow/boot.go:154:		log.G(ctx).Debug("configuring UEFI boot")
internal/builder/vm/lcow/boot.go:165:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/confidential.go:41:	log.G(ctx).Debug("parseConfidentialOptions: starting confidential options parsing")
internal/builder/vm/lcow/confidential.go:65:	log.G(ctx).WithField("vmgsTemplatePath", vmgsTemplatePath).Debug("VMGS template path configured")
internal/builder/vm/lcow/confidential.go:81:	log.G(ctx).WithField("dmVerityRootfsPath", dmVerityRootfsTemplatePath).Debug("DM Verity rootfs path configured")
internal/builder/vm/lcow/confidential.go:89:	log.G(ctx).Debug("configuring UEFI secure boot for confidential VM")
internal/builder/vm/lcow/confidential.go:100:	log.G(ctx).Debug("creating security policy digest")
internal/builder/vm/lcow/confidential.go:125:	log.G(ctx).Debug("configuring HvSocket service table for confidential VM")
internal/builder/vm/lcow/confidential.go:141:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/confidential.go:160:	log.G(ctx).Debug("parseConfidentialOptions completed successfully")
internal/builder/vm/lcow/confidential.go:172:	log.G(ctx).Debug("setGuestState: starting guest state configuration")
internal/builder/vm/lcow/confidential.go:177:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/confidential.go:189:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/confidential.go:203:	log.G(ctx).Debug("granting VM group access to confidential files")
internal/builder/vm/lcow/confidential.go:228:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/confidential.go:242:	log.G(ctx).Debug("setGuestState completed successfully")
internal/builder/vm/lcow/devices.go:38:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/devices.go:61:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/devices.go:96:			log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/devices.go:112:	log.G(ctx).WithField("scsiControllerCount", scsiControllerCount).Debug("configuring SCSI controllers")
internal/builder/vm/lcow/devices.go:131:		log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/devices.go:145:		log.G(ctx).Debug("parsing vPCI device assignments")
internal/builder/vm/lcow/devices.go:162:				log.G(ctx).Debug("NUMA affinity propagation enabled for vPCI devices")
internal/builder/vm/lcow/devices.go:187:				log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/devices.go:196:	log.G(ctx).Debug("parseDeviceOptions completed successfully")
internal/builder/vm/lcow/devices.go:204:		log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/devices.go:215:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/kernel_args.go:32:	log.G(ctx).WithField("rootFsFile", rootFsFile).Debug("buildKernelArgs: starting kernel arguments construction")
internal/builder/vm/lcow/kernel_args.go:88:	log.G(ctx).WithField("kernelArgs", result).Debug("buildKernelArgs completed successfully")
internal/builder/vm/lcow/kernel_args.go:100:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/kernel_args.go:161:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/kernel_args.go:179:			log.G(ctx).Warn("ignoring `WritableOverlayDirs` option since rootfs is already writable")
internal/builder/vm/lcow/kernel_args.go:194:	log.G(ctx).Debug("buildInitArgs completed successfully")
internal/builder/vm/lcow/specs.go:36:	log.G(ctx).Info("BuildSandboxConfig: starting sandbox spec generation")
internal/builder/vm/lcow/specs.go:54:	log.G(ctx).WithField("platform", platform).Debug("validating sandbox platform")
internal/builder/vm/lcow/specs.go:194:		log.G(ctx).Debug("disabling memory overcommit for confidential VM")
internal/builder/vm/lcow/specs.go:226:		log.G(ctx).WithField("kernelArgs", kernelArgs).Debug("kernel arguments configured")
internal/builder/vm/lcow/specs.go:233:	log.G(ctx).Debug("assembling final sandbox hcs spec")
internal/builder/vm/lcow/specs.go:272:	log.G(ctx).Info("sandbox spec generation completed successfully")
internal/builder/vm/lcow/specs.go:279:	log.G(ctx).WithField("annotationCount", len(annotations)).Debug("processAnnotations: starting annotations processing")
internal/builder/vm/lcow/specs.go:300:	log.G(ctx).Debug("processAnnotations completed successfully")
internal/builder/vm/lcow/specs.go:308:	log.G(ctx).WithField("platform", platform).Debug("parseSandboxOptions: starting sandbox options parsing")
internal/builder/vm/lcow/specs.go:338:		log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/specs.go:348:	log.G(ctx).Debug("parseSandboxOptions completed successfully")
internal/builder/vm/lcow/specs.go:354:	log.G(ctx).Debug("parseStorageQOSOptions: starting storage QOS options parsing")
internal/builder/vm/lcow/specs.go:359:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/specs.go:376:	log.G(ctx).Debug("setAdditionalOptions: starting additional options parsing")
internal/builder/vm/lcow/specs.go:406:	log.G(ctx).Debug("setAdditionalOptions completed successfully")
internal/builder/vm/lcow/topology.go:24:	log.G(ctx).Debug("parseCPUOptions: starting CPU options parsing")
internal/builder/vm/lcow/topology.go:62:		log.G(ctx).WithField("resourcePartitionID", resourcePartitionID).Debug("setting resource partition ID")
internal/builder/vm/lcow/topology.go:74:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/topology.go:86:	log.G(ctx).Debug("parseMemoryOptions: starting memory options parsing")
internal/builder/vm/lcow/topology.go:116:	log.G(ctx).WithFields(logrus.Fields{
internal/builder/vm/lcow/topology.go:130:	log.G(ctx).Debug("parseNUMAOptions: starting NUMA options parsing")
internal/builder/vm/lcow/topology.go:149:		log.G(ctx).WithField("virtualNodeCount", hcsNuma.VirtualNodeCount).Debug("vNUMA topology configured")
internal/builder/vm/lcow/topology.go:158:	log.G(ctx).WithFields(logrus.Fields{
internal/cmd/cmd.go:135:	cmd.Log = log.G(ctx)
internal/cmd/diag.go:35:	cmd.Log = log.G(ctx)
internal/cmd/io_binary.go:70:			log.G(ctx).WithError(err).Errorf("error closing wait pipe: %s", waitPipePath)
internal/cmd/io_binary.go:110:	log.G(ctx).WithFields(logrus.Fields{
internal/cmd/io_binary.go:186:				log.G(ctx).WithError(err).Errorf("error while closing stdout npipe")
internal/cmd/io_binary.go:192:				log.G(ctx).WithError(err).Errorf("error while closing stderr npipe")
internal/cmd/io_binary.go:205:				log.G(ctx).WithError(err).Errorf("error while waiting for binary cmd to finish")
internal/cmd/io_binary.go:208:			log.G(ctx).Errorf("timeout while waiting for binaryIO process to finish. Killing")
internal/cmd/io_binary.go:211:				log.G(ctx).WithError(err).Errorf("error while killing binaryIO process")
internal/cmd/io_npipe.go:26:	log.G(ctx).WithFields(logrus.Fields{
internal/cmd/io_npipe.go:197:			log.G(ctx).Debug("npipeio::sinCloser")
internal/cmd/io_npipe.go:203:			log.G(ctx).Debug("npipeio::outErrCloser - stdout")
internal/cmd/io_npipe.go:207:			log.G(ctx).Debug("npipeio::outErrCloser - stderr")
internal/cmd/io_npipe.go:216:			log.G(ctx).Debug("npipeio::sinCloser")
internal/computecore/computecore.go:137:		log.G(ctx).WithFields(logrus.Fields{
internal/computecore/computecore.go:150:			log.G(ctx).WithField(logfields.Timeout, trueTimeout).
internal/controller/vm/vm.go:152:	log.G(ctx).Debugf("using gcs connection timeout: %s\n", timeout.GCSConnectionTimeout)
internal/controller/vm/vm.go:220:		log.G(ctx).WithField("currentState", c.vmState).Debug("waitForVMExit: state transition to Terminated was a no-op")
internal/controller/vm/vm.go:398:		log.G(ctx).Errorf("close guest connection failed: %s", err)
internal/credentials/credentials.go:51:	log.G(ctx).WithField("containerID", id).Debug("creating container credential guard instance")
internal/credentials/credentials.go:118:	log.G(ctx).WithField("containerID", id).Debug("removing container credential guard")
internal/devices/assigned_devices.go:37:				log.G(ctx).WithError(releaseErr).Error("failed to release container resource")
internal/devices/assigned_devices.go:49:		log.G(ctx).WithField("vmbus id", vmBusInstanceID).Info("vmbus instance ID")
internal/devices/drivers.go:37:				log.G(ctx).WithError(releaseErr).Error("failed to release container resource")
internal/devices/pnp.go:60:		log.G(ctx).WithFields(logrus.Fields{
internal/devices/pnp.go:67:	log.G(ctx).WithField("added drivers", driverDir).Debug("installed drivers")
internal/gcs-sidecar/bridge.go:349:					log.G(ctx).WithError(err).Error("failed to send request to shimRequestChan")
internal/gcs-sidecar/bridge.go:455:						log.G(ctx).WithError(err).Error("failed to unmarshal the request")
internal/gcs-sidecar/bridge.go:473:					log.G(ctx).WithError(err).Error("failed to send request to b.sendToShimCh")
internal/gcs-sidecar/handlers.go:70:		log.G(ctx).Tracef("createContainer: uvmConfig: {systemType: %v, timeZoneInformation: %v}}", systemType, timeZoneInformation)
internal/gcs-sidecar/handlers.go:75:		log.G(ctx).Tracef("rpcCreate: HostedSystemConfig: {schemaVersion: %v, container: %v}}", schemaVersion, container)
internal/gcs-sidecar/handlers.go:83:		log.G(ctx).Tracef("rpcCreate: CWCOWHostedSystemConfig {spec: %v, schemaVersion: %v, container: %v}}", string(req.message), schemaVersion, container)
internal/gcs-sidecar/handlers.go:87:			log.G(ctx).Trace("Container has registry changes, validating against policy")
internal/gcs-sidecar/handlers.go:97:						log.G(ctx).WithField("name", value.Name).Trace("Registry value matches default, accepting without policy check")
internal/gcs-sidecar/handlers.go:106:				log.G(ctx).Tracef("Validating %d registry values against policy", len(nonDefaultValues))
internal/gcs-sidecar/handlers.go:114:					log.G(ctx).WithError(err).Warn("Registry changes validation failed - rejecting")
internal/gcs-sidecar/handlers.go:117:				log.G(ctx).Tracef("All container registry values validated successfully")
internal/gcs-sidecar/handlers.go:120:			log.G(ctx).Infof("Registry validation complete: %d total values (%d defaults + %d validated)",
internal/gcs-sidecar/handlers.go:142:		log.G(ctx).Tracef("Adding ContainerID: %v", containerID)
internal/gcs-sidecar/handlers.go:144:			log.G(ctx).Tracef("Container exists in the map.")
internal/gcs-sidecar/handlers.go:150:					log.G(ctx).WithError(removeErr).Errorf("Failed to remove container: %v", containerID)
internal/gcs-sidecar/handlers.go:186:		log.G(ctx).Tracef("marshaled request buffer: %s", string(buf))
internal/gcs-sidecar/handlers.go:612:	log.G(ctx).Tracef("modifySettings: MsgType: %v, Payload: %v", req.header.Type, string(req.message))
internal/gcs-sidecar/handlers.go:620:	log.G(ctx).Tracef("modifySettings: resourceType: %v, requestType: %v", guestResourceType, guestRequestType)
internal/gcs-sidecar/handlers.go:639:			log.G(ctx).Tracef("WCOWCombinedLayers: {%v}", settings)
internal/gcs-sidecar/handlers.go:643:			log.G(ctx).Tracef("HostComputeNamespaces { %v}", settings)
internal/gcs-sidecar/handlers.go:647:			log.G(ctx).Tracef("NetworkModifyRequest { %v}", settings)
internal/gcs-sidecar/handlers.go:651:			log.G(ctx).Tracef("wcowMappedVirtualDisk { %v}", wcowMappedVirtualDisk)
internal/gcs-sidecar/handlers.go:655:			log.G(ctx).Tracef("hvSocketAddress { %v }", hvSocketAddress)
internal/gcs-sidecar/handlers.go:659:			log.G(ctx).Tracef("hcsschema.MappedDirectory { %v }", settings)
internal/gcs-sidecar/handlers.go:663:			log.G(ctx).Tracef("WCOWConfidentialOptions: { %v}", securityPolicyRequest)
internal/gcs-sidecar/handlers.go:701:				log.G(ctx).Tracef("WCOWBlockCIMMounts Add { %v}", wcowBlockCimMounts)
internal/gcs-sidecar/handlers.go:729:							log.G(ctx).WithFields(map[string]interface{}{
internal/gcs-sidecar/handlers.go:752:					log.G(ctx).Debugf("block CIM layer digest %s, path: %s\n", layerHashes[i], physicalDevPath)
internal/gcs-sidecar/handlers.go:774:				log.G(ctx).Tracef("Cached %d verified CIM layer hashes for volume %s (container %s)", len(hashesToVerify), volGUID, containerID)
internal/gcs-sidecar/handlers.go:789:				log.G(ctx).Tracef("WCOWBlockCIMMounts: Remove")
internal/gcs-sidecar/handlers.go:811:			log.G(ctx).Tracef("ResourceTypeMappedVirtualDiskForContainerScratch: { %v }", wcowMappedVirtualDisk)
internal/gcs-sidecar/handlers.go:833:					log.G(ctx).Tracef("DiskNumber of lun %d is:  %d", wcowMappedVirtualDisk.Lun, devNumber)
internal/gcs-sidecar/handlers.go:838:			log.G(ctx).Tracef("diskPath: %v, diskNumber: %v ", diskPath, devNumber)
internal/gcs-sidecar/handlers.go:843:			log.G(ctx).Tracef("mountedVolumePath returned from InvokeFsFormatter: %v", mountedVolumePath)
internal/gcs-sidecar/handlers.go:866:				log.G(ctx).Tracef("CWCOWCombinedLayers:: ContainerID: %v, ContainerRootPath: %v, Layers: %v, ScratchPath: %v",
internal/gcs-sidecar/handlers.go:889:						log.G(ctx).Tracef("Verified CIM hashes for reused mount volume %s (container %s)", volGUID.String(), containerID)
internal/gcs-sidecar/handlers.go:917:				log.G(ctx).Tracef("CWCOWCombinedLayers: Remove")
internal/gcs-sidecar/host.go:70:		log.G(ctx).Tracef("Container exists in the map: %v", ok)
internal/gcs-sidecar/host.go:73:	log.G(ctx).Tracef("AddContainer: ID: %v", id)
internal/gcs-sidecar/host.go:84:		log.G(ctx).Tracef("RemoveContainer: Container not found: ID: %v", id)
internal/gcs-sidecar/host.go:98:		log.G(ctx).Tracef("GetCreatedContainer: Container not found: ID: %v", id)
internal/gcs-sidecar/uvm.go:35:			log.G(ctx).Info("Unmarshalling log forward service modify settings")
internal/gcs-sidecar/uvm.go:43:			log.G(ctx).Errorf("Invalid ServiceModificationRequest: %v", serviceModifyRequest.PropertyType)
internal/gcs-sidecar/uvm.go:155:			log.G(ctx).Errorf("invalid modifySettingsRequest: %v", modifyGuestSettingsRequest.ResourceType)
internal/gcs/container.go:202:			log.G(ctx).WithError(err).Warn("ignoring missing container")
internal/gcs/container.go:263:	log.G(ctx).Debug("container exited")
internal/gcs/guestconnection.go:323:					log.G(ctx).WithError(err).Warn("failed to encode OpenCensus Tracestate")
internal/gcs/process.go:110:	log.G(ctx).WithField("pid", p.id).Debug("created process pid")
internal/gcs/process.go:135:		log.G(ctx).WithError(err).Warn("close stdin failed")
internal/gcs/process.go:138:		log.G(ctx).WithError(err).Warn("close stdout failed")
internal/gcs/process.go:141:		log.G(ctx).WithError(err).Warn("close stderr failed")
internal/gcs/process.go:257:			log.G(ctx).WithFields(logrus.Fields{
internal/gcs/process.go:290:		log.G(ctx).WithError(err).Error("failed wait")
internal/gcs/process.go:292:	log.G(ctx).WithField("exitCode", ec).Debug("process exited")
internal/guest/bridge/bridge.go:311:				entry := log.G(ctx)
internal/guest/bridge/bridge_v2.go:202:	log.G(ctx).WithField("pid", pid).Debug("created process pid")
internal/guest/network/netns.go:197:		log.G(ctx).WithFields(logrus.Fields{
internal/guest/network/netns.go:210:		log.G(ctx).WithFields(logrus.Fields{
internal/guest/network/netns.go:240:		log.G(ctx).WithField("route", r).Debugf("adding a route to interface %s", link.Attrs().Name)
internal/guest/network/netns.go:288:				log.G(ctx).Infof("gw is outside of the subnet: %v", gw)
internal/guest/network/network.go:142:	log.G(ctx).WithField("ifname", ifname).Debug("resolved ifname")
internal/guest/runtime/hcsv2/container.go:83:	entity := log.G(ctx).WithField(logfields.ContainerID, c.id)
internal/guest/runtime/hcsv2/container.go:136:	log.G(ctx).WithField(logfields.ContainerID, c.id).Info("opengcs::Container::ExecProcess")
internal/guest/runtime/hcsv2/container.go:203:	log.G(ctx).WithField(logfields.ContainerID, c.id).Info("opengcs::Container::GetAllProcessPids")
internal/guest/runtime/hcsv2/container.go:217:	log.G(ctx).WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/container.go:230:	entity := log.G(ctx).WithField(logfields.ContainerID, c.id)
internal/guest/runtime/hcsv2/container.go:280:	log.G(ctx).WithField(logfields.ContainerID, c.id).Info("opengcs::Container::Update")
internal/guest/runtime/hcsv2/nvidia_utils.go:75:		log.G(ctx).WithField("hook", log.Format(ctx, nvidiaHook)).Debug("adding nvidia device runtime hook")
internal/guest/runtime/hcsv2/process.go:99:			log.G(ctx).WithError(err).Error("failed to wait for runc process")
internal/guest/runtime/hcsv2/process.go:102:		log.G(ctx).WithField("exitCode", p.exitCode).Debug("process exited")
internal/guest/runtime/hcsv2/process.go:113:				log.G(ctx).WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/process.go:198:			log.G(ctx).Debug("wait completed, releasing wait count")
internal/guest/runtime/hcsv2/process.go:205:				log.G(ctx).Debug("first wait completed, releasing first wait count")
internal/guest/runtime/hcsv2/process.go:218:			log.G(ctx).Debug("wait canceled before exit, releasing wait count")
internal/guest/runtime/hcsv2/process.go:244:		log.G(ctx).WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/sandbox_container.go:93:		log.G(ctx).Infof("setupSandboxContainerSpec: Did not find NS spec %v, err %v", spec, err)
internal/guest/runtime/hcsv2/uvm.go:469:					log.G(ctx).WithError(err).Debug("failed to add SEV device")
internal/guest/runtime/hcsv2/uvm.go:569:		log.G(ctx).WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/uvm.go:660:		entry := log.G(ctx).WithField(logfields.Path, configFile)
internal/guest/runtime/hcsv2/uvm.go:665:			log.G(ctx).WithField(
internal/guest/runtime/hcsv2/uvm.go:986:				log.G(ctx).WithField("stats", log.Format(ctx, cgroupMetrics)).Trace("queried cgroup statistics")
internal/guest/runtime/hcsv2/uvm.go:990:			log.G(ctx).WithField("propertyType", requestedProperty).Warn("unknown or empty property type")
internal/guest/spec/spec.go:467:		log.G(ctx).WithField("sizeKB", val).Debug("set custom /dev/shm size")
internal/guest/spec/spec.go:475:			log.G(ctx).WithFields(logrus.Fields{
internal/guest/spec/spec.go:484:		log.G(ctx).Debugf("'%s' set for privileged container", annotations.LCOWPrivileged)
internal/guest/spec/spec_devices.go:52:		entry := log.G(ctx).WithField("windows-device", log.Format(ctx, d))
internal/guest/spec/spec_devices.go:116:			entry := log.G(ctx).WithField("host-device", log.Format(ctx, d))
internal/guest/spec/spec_devices.go:180:			log.G(ctx).Warnf("The same type '%s', major '%d' and minor '%d', should not be used for multiple devices.", dev.Type, dev.Major, dev.Minor)
internal/guest/storage/crypt/crypt.go:51:			log.G(ctx).WithError(err).WithFields(logrus.Fields{
internal/guest/storage/crypt/crypt.go:59:						log.G(ctx).WithError(err).Warning("cryptsetup failed, context timeout")
internal/guest/storage/crypt/crypt.go:157:			log.G(ctx).WithError(err).Debugf("failed to delete temporary folder: %s", tempDir)
internal/guest/storage/crypt/crypt.go:180:				log.G(ctx).WithError(inErr).Debug("failed to cleanup crypt device")
internal/guest/storage/devicemapper/devicemapper.go:237:		log.G(ctx).WithError(err).Warning("CreateDevice error")
internal/guest/storage/devicemapper/devicemapper.go:248:					log.G(ctx).WithError(err).Error("CreateDeviceWithRetryErrors failed, context timeout")
internal/guest/storage/overlay/overlay.go:36:		log.G(ctx).WithError(statErr).WithField("path", filepath.Dir(path)).Warn("failed to get disk information for ENOSPC error")
internal/guest/storage/overlay/overlay.go:49:	log.G(ctx).WithFields(logrus.Fields{
internal/guest/storage/pmem/pmem.go:47:				log.G(ctx).WithError(err).Debugf("error cleaning up target: %s", target)
internal/guest/storage/pmem/pmem.go:151:			log.G(ctx).WithError(err).Debugf("failed to remove dm verity target: %s", dmVerityName)
internal/guest/storage/pmem/pmem.go:159:			log.G(ctx).WithError(err).Debugf("failed to remove dm linear target: %s", dmLinearName)
internal/guest/storage/scsi/scsi.go:176:		log.G(ctx).WithFields(logrus.Fields{
internal/guest/storage/scsi/scsi.go:222:			log.G(ctx).WithError(err).Debug("get device filesystem failed, retrying in 500ms")
internal/guest/storage/scsi/scsi.go:230:		log.G(ctx).WithField("filesystem", deviceFS).Debug("filesystem found on device")
internal/guest/storage/scsi/scsi.go:302:		log.G(ctx).WithField("target", target).Trace("removing block device symlink")
internal/guest/storage/scsi/scsi.go:318:			log.G(ctx).WithError(err).Debugf("failed to remove dm verity target: %s", dmVerityName)
internal/guest/storage/scsi/scsi.go:365:					log.G(ctx).WithField("blockPath", blockPath).Warn(
internal/guest/storage/scsi/scsi.go:412:	log.G(ctx).WithField("devicePath", devicePath).Debug("found device path")
internal/guest/storage/scsi/scsi.go:422:					log.G(ctx).Warnf("context timed out while retrying to find device %s: %v", devicePath, err)
internal/guest/storage/scsi/scsi.go:432:			log.G(ctx).WithError(err).Warnf("failed to close file: %s", devicePath)
internal/hcs/errors.go:136:			log.G(ctx).WithError(err).Warning("Could not unmarshal HCS result")
internal/hcs/process.go:77:					log.G(ctx).WithError(err).Warn("force unblocking process waits")
internal/hcs/process.go:150:		log.G(ctx).WithField("err", err).Error("OpenComputeSystem() call failed")
internal/hcs/process.go:154:			log.G(ctx).WithField("err", err).Error("Terminate() call failed")
internal/hcs/process.go:230:		log.G(ctx).WithError(err).Error("failed wait")
internal/hcs/process.go:248:						log.G(ctx).WithField("wait-result", properties.LastWaitResult).Warning("non-zero last wait result")
internal/hcs/process.go:256:	log.G(ctx).WithField("exitCode", exitCode).Debug("process exited")
internal/hcs/system.go:286:		log.G(ctx).Debug("system exited")
internal/hcs/system.go:288:		log.G(ctx).Debug("unexpected system exit")
internal/hcs/system.go:414:					log.G(ctx).WithError(err).Warn("failed to get statistics in-proc")
internal/hcs/system.go:542:	logEntry := log.G(ctx)
internal/hcs/system.go:691:	log.G(ctx).WithField("pid", processInfo.ProcessId).Debug("created process pid")
internal/hcs/waithelper.go:37:		log.G(ctx).WithField("callbackNumber", callbackNumber).Error("failed to waitForNotification: callbackNumber does not exist in callbackMap")
internal/hcs/waithelper.go:45:		log.G(ctx).WithField("type", expectedNotification).Error("unknown notification type in waitForNotification")
internal/hcsoci/create.go:131:	log.G(ctx).WithFields(logrus.Fields{
internal/hcsoci/create.go:244:	log.G(ctx).Debug("hcsshim::CreateContainer allocating resources")
internal/hcsoci/create.go:249:		log.G(ctx).Debug("hcsshim::CreateContainer allocateLinuxResources")
internal/hcsoci/create.go:252:			log.G(ctx).WithError(err).Debug("failed to allocateLinuxResources")
internal/hcsoci/create.go:257:			log.G(ctx).WithError(err).Debug("failed createHCSContainerDocument")
internal/hcsoci/create.go:263:			log.G(ctx).WithError(err).Debug("failed to allocateWindowsResources")
internal/hcsoci/create.go:266:		log.G(ctx).Debug("hcsshim::CreateContainer creating container document")
internal/hcsoci/create.go:269:			log.G(ctx).WithError(err).Debug("failed createHCSContainerDocument")
internal/hcsoci/create.go:304:	log.G(ctx).Debug("hcsshim::CreateContainer creating compute system")
internal/hcsoci/create.go:312:			log.G(ctx).Debug("redirecting container HvSocket for WCOW")
internal/hcsoci/devices.go:110:					log.G(ctx).WithError(releaseErr).Error("failed to release container resource")
internal/hcsoci/devices.go:144:			log.G(ctx).WithField("parsed devices", specDev).Info("added windows device to spec")
internal/hcsoci/devices.go:167:					log.G(ctx).WithError(releaseErr).Error("failed to release container resource")
internal/hcsoci/devices.go:204:					log.G(ctx).WithError(releaseErr).Error("failed to release container resource")
internal/hcsoci/hcsdoc_lcow.go:110:	log.G(ctx).WithField("guestRoot", guestRoot).Debug("hcsshim::createLinuxContainerDoc")
internal/hcsoci/hcsdoc_wcow.go:143:	log.G(ctx).Debug("hcsshim: CreateHCSContainerDocument")
internal/hcsoci/hcsdoc_wcow.go:222:		log.G(ctx).WithFields(logrus.Fields{
internal/hcsoci/hcsdoc_wcow.go:246:			log.G(ctx).WithFields(logrus.Fields{
internal/hcsoci/hcsdoc_wcow.go:493:		log.G(ctx).WithField("count", len(testAnnotationValues)).Info("adding test annotation registry values to container")
internal/hcsoci/hcsdoc_wcow.go:496:		log.G(ctx).Debug("no test annotation registry values found in container annotations")
internal/hcsoci/hcsdoc_wcow.go:526:		log.G(ctx).WithField("hcsv2 device", v2Dev).Debug("adding assigned device to container doc")
internal/hcsoci/network.go:18:	l := log.G(ctx).WithField(logfields.ContainerID, coi.ID)
internal/hcsoci/network.go:29:	log.G(ctx).WithFields(logrus.Fields{
internal/hcsoci/network.go:43:		log.G(ctx).WithFields(logrus.Fields{
internal/hcsoci/resources.go:25:		log.G(ctx).WithFields(logrus.Fields{
internal/hcsoci/resources.go:40:		log.G(ctx).WithFields(logrus.Fields{
internal/hcsoci/resources_lcow.go:32:		log.G(ctx).Debug("hcsshim::allocateLinuxResources mounting storage")
internal/hcsoci/resources_lcow.go:86:			l := log.G(ctx).WithField("mount", fmt.Sprintf("%+v", mount))
internal/hcsoci/resources_wcow.go:39:			log.G(ctx).Debug("hcsshim::allocateWindowsResources mounting storage")
internal/hcsoci/resources_wcow.go:149:		l := log.G(ctx).WithField("mount", fmt.Sprintf("%+v", mount))
internal/jobcontainers/jobcontainer.go:102:	log.G(ctx).WithField("id", id).Debug("Creating job container")
internal/jobcontainers/jobcontainer.go:475:	log.G(ctx).WithField("id", c.id).Debug("shutting down job container")
internal/jobcontainers/jobcontainer.go:499:			log.G(ctx).WithField("pid", pid).Error("failed to signal process in job container")
internal/jobcontainers/jobcontainer.go:604:	log.G(ctx).WithField("id", c.id).Debug("terminating job container")
internal/jobcontainers/jobcontainer.go:657:			log.G(ctx).WithError(err).Warn("error while polling for job container notification")
internal/jobcontainers/jobcontainer.go:667:			log.G(ctx).WithField("message", msg).Warn("unknown job object notification encountered")
internal/jobcontainers/logon.go:77:	log.G(ctx).WithField("username", user).Debug("Created local user account for job container")
internal/jobcontainers/mounts.go:104:			log.G(ctx).WithFields(logrus.Fields{
internal/jobcontainers/mounts.go:133:			log.G(ctx).WithError(err).Warnf("failed to setup symlink from %s to containers rootfs at %s", mount.Source, fullCtrPath)
internal/jobcontainers/process.go:161:	log.G(ctx).WithField("pid", p.Pid()).Debug("waitBackground for JobProcess")
internal/jobcontainers/process.go:231:	log.G(ctx).WithField("pid", p.Pid()).Debug("killing job process")
internal/jobcontainers/storage.go:45:		log.G(ctx).Debug("mounting job container storage")
internal/jobcontainers/storage.go:54:					log.G(ctx).WithError(closeErr).Errorf("failed to cleanup mounted layers during another failure(%s)", err)
internal/jobobject/iocp.go:46:			log.G(ctx).WithError(err).Error("failed to poll for job object message")
internal/jobobject/iocp.go:52:				log.G(ctx).WithField("value", msq).Warn("encountered non queue type in job map")
internal/jobobject/iocp.go:57:				log.G(ctx).WithFields(logrus.Fields{
internal/jobobject/iocp.go:69:				log.G(ctx).WithFields(logrus.Fields{
internal/jobobject/iocp.go:76:			log.G(ctx).Warn("received a message for a job not present in the mapping")
internal/layers/lcow.go:51:		log.G(ctx).WithError(err).Error("failed RemoveCombinedLayersLCOW")
internal/layers/lcow.go:57:		log.G(ctx).WithError(err).Error("failed LCOW scratch mount release")
internal/layers/lcow.go:64:			log.G(ctx).WithFields(logrus.Fields{
internal/layers/lcow.go:97:	log.G(ctx).WithField("os", vm.OS()).Debug("hcsshim::MountLCOWLayers V2 UVM")
internal/layers/lcow.go:107:					log.G(ctx).WithError(err).Warn("failed to remove lcow layer on cleanup")
internal/layers/lcow.go:114:		log.G(ctx).WithField("layerPath", layer.VHDPath).Debug("mounting layer")
internal/layers/lcow.go:134:	log.G(ctx).WithField("hostPath", hostPath).Debug("mounting scratch VHD")
internal/layers/lcow.go:169:				log.G(ctx).WithError(err).Warn("failed to remove scratch on cleanup")
internal/layers/lcow.go:179:	log.G(ctx).Debug("hcsshim::MountLCOWLayers Succeeded")
internal/layers/lcow.go:198:			log.G(ctx).WithFields(logrus.Fields{
internal/layers/lcow.go:222:	log.G(ctx).WithFields(logrus.Fields{
internal/layers/wcow_mount.go:165:					log.G(ctx).WithField("path", l.scratchLayerPath).WithError(hcserr.Err).Warning("retrying layer operations after failure")
internal/layers/wcow_mount.go:236:				log.G(ctx).WithError(err).Warnf("mount process isolated cim layers common, undo failed with: %s", rErr)
internal/layers/wcow_mount.go:252:	log.G(ctx).WithFields(logrus.Fields{
internal/layers/wcow_mount.go:281:	log.G(ctx).WithField("layer data", layerData).Debug("unionFS filter attached")
internal/layers/wcow_mount.go:303:				log.G(ctx).WithError(err).Warnf("mount process isolated forked CIM layers, undo failed with: %s", rErr)
internal/layers/wcow_mount.go:336:				log.G(ctx).WithError(err).Warnf("mount process isolated forked CIM layers, undo failed with: %s", rErr)
internal/layers/wcow_mount.go:341:	log.G(ctx).WithFields(logrus.Fields{
internal/layers/wcow_mount.go:359:	log.G(ctx).WithField("volume", volume).Debug("mounted blockCIM layers for process isolated container")
internal/layers/wcow_mount.go:379:		log.G(ctx).WithError(err).Error("failed RemoveCombinedLayersWCOW")
internal/layers/wcow_mount.go:385:		log.G(ctx).WithError(err).Error("failed WCOW scratch mount release")
internal/layers/wcow_mount.go:392:			log.G(ctx).WithFields(logrus.Fields{
internal/layers/wcow_mount.go:405:	log.G(ctx).WithField("os", vm.OS()).Debug("hcsshim::MountWCOWLayers V2 UVM")
internal/layers/wcow_mount.go:421:					log.G(ctx).WithError(err).Warn("failed to remove wcow layer on cleanup")
internal/layers/wcow_mount.go:428:		log.G(ctx).WithField("layerPath", layerPath).Debug("mounting layer")
internal/layers/wcow_mount.go:440:	log.G(ctx).WithField("hostPath", hostPath).Debug("mounting scratch VHD")
internal/layers/wcow_mount.go:451:				log.G(ctx).WithError(err).Warn("failed to remove scratch on cleanup")
internal/layers/wcow_mount.go:486:	log.G(ctx).Debug("hcsshim::MountWCOWLayers Succeeded")
internal/layers/wcow_mount.go:507:				log.G(ctx).WithError(err).Warnf("mount process isolated forked CIM layers, undo failed with: %s", rErr)
internal/layers/wcow_mount.go:512:	log.G(ctx).WithFields(logrus.Fields{
internal/layers/wcow_mount.go:525:	log.G(ctx).WithField("volume", mountedCIMs.MountedVolumePath()).Debug("mounted blockCIM layers for hyperV isolated container")
internal/layers/wcow_mount.go:539:	log.G(ctx).WithFields(logrus.Fields{
internal/layers/wcow_mount.go:570:	log.G(ctx).Debug("hcsshim::mountHyperVIsolatedBlockCIMLayers Succeeded")
internal/layers/wcow_mount.go:582:	log.G(ctx).WithFields(logrus.Fields{
internal/layers/wcow_mount.go:612:	log.G(ctx).WithFields(logrus.Fields{
internal/lcow/common.go:48:	log.G(ctx).WithFields(logrus.Fields{
internal/lcow/common.go:64:	log.G(ctx).WithField("dest", destPath).Debug("lcow::FormatDisk complete")
internal/lcow/disk.go:29:	log.G(ctx).WithFields(logrus.Fields{
internal/lcow/disk.go:43:	log.G(ctx).WithFields(logrus.Fields{
internal/lcow/disk.go:52:	log.G(ctx).WithField("dest", destPath).Debug("lcow::FormatDisk complete")
internal/lcow/scratch.go:47:	log.G(ctx).WithFields(logrus.Fields{
internal/lcow/scratch.go:59:			log.G(ctx).WithFields(logrus.Fields{
internal/lcow/scratch.go:93:	log.G(ctx).WithFields(logrus.Fields{
internal/lcow/scratch.go:108:		log.G(ctx).WithError(err).WithField("stderr", mkfsStderr.String()).Error("mkfs.ext4 failed")
internal/lcow/scratch.go:125:	log.G(ctx).WithField("dest", destFile).Debug("lcow::CreateScratch created (non-cache)")
internal/log/context.go:90:	ctx = context.WithValue(ctx, _entryContextKey, entry)
internal/oci/annotations.go:46:					log.G(ctx).WithFields(logrus.Fields{
internal/oci/annotations.go:69:		log.G(ctx).WithFields(logrus.Fields{
internal/oci/annotations.go:118:		entry := log.G(ctx).WithFields(logrus.Fields{
internal/oci/annotations.go:230:		entry := log.G(ctx).WithFields(logrus.Fields{
internal/oci/annotations.go:405:	entry := log.G(ctx).WithFields(logrus.Fields{
internal/oci/uvm.go:140:			log.G(ctx).WithFields(logrus.Fields{
internal/oci/uvm.go:294:			log.G(ctx).WithFields(logrus.Fields{
internal/resources/resources.go:121:				log.G(ctx).Warn(err)
internal/resources/resources.go:137:					log.G(ctx).WithError(err).Error("failed to release container resource")
internal/resources/resources.go:144:				log.G(ctx).WithError(err).Error("failed to release container resource")
internal/resources/resources.go:153:					log.G(ctx).WithError(err).Error("failed to release container resource")
internal/shim/shim.go:178:	l := log.G(ctx)
internal/shim/shim.go:188:	return log.WithLogger(ctx, l), nil
internal/shim/shim.go:198:	ctx = log.WithLogger(ctx, log.G(ctx).WithField("runtime", manager.Name()))
internal/shim/shim.go:254:	ctx = context.WithValue(ctx, OptsKey{}, Opts{BundlePath: bundlePath, Debug: debugFlag})
internal/shim/shim.go:261:		logger := log.G(ctx).WithFields(log.Fields{
internal/shim/shim.go:354:		log.G(ctx).WithFields(log.Fields{"id": pID, "type": p.Type}).Debug("loading plugin")
internal/shim/shim.go:388:				log.G(ctx).WithFields(log.Fields{"id": pID, "type": p.Type, "error": err}).Info("skip loading plugin")
internal/shim/shim.go:395:			log.G(ctx).WithField("id", pID).Debug("registering ttrpc service")
internal/shim/shim.go:466:			log.G(ctx).WithError(err).Fatal("containerd-shim: ttrpc server failure")
internal/shim/shim.go:481:			log.G(ctx).WithError(err).Warn("Could not setup pprof")
internal/shim/shim.go:485:	logger := log.G(ctx).WithFields(log.Fields{
internal/shim/shim.go:527:			log.G(ctx).WithError(err).Fatal("containerd-shim: pprof endpoint failure")
internal/shim/shim_test.go:51:	ctx = context.WithValue(ctx, OptsKey{}, Opts{Debug: true})
internal/shim/util_test.go:37:	callCtx := context.WithValue(context.Background(), callKey{}, callValue)
internal/shim/util_test.go:65:		ctx = context.WithValue(ctx, firstKey{}, firstValue)
internal/shim/util_test.go:95:		ctx = context.WithValue(ctx, secondKey{}, secondValue)
internal/tools/networkagent/main.go:144:	log.G(ctx).WithField("endpt", endpt).Info("ConfigureContainerNetworking created endpoint")
internal/tools/networkagent/main.go:225:	log.G(ctx).WithField("endpt", endpt).Info("ConfigureContainerNetworking created endpoint")
internal/tools/networkagent/main.go:264:			log.G(ctx).Warn("failed to find endpoint to delete")
internal/tools/networkagent/main.go:268:			log.G(ctx).WithField("name", endpoint.ID).Warn("failed to get endpoint settings")
internal/tools/networkagent/main.go:279:				log.G(ctx).WithField("name", endpoint.ID).Warn("invalid endpoint settings type")
internal/tools/networkagent/main.go:286:				log.G(ctx).WithField("name", endpointName).Warn("failed to delete endpoint")
internal/tools/networkagent/main.go:297:				log.G(ctx).WithField("name", networkName).Warn("failed to delete network")
internal/tools/networkagent/main.go:308:	log.G(ctx).WithField("req", req).Info("ConfigureContainerNetworking request")
internal/tools/networkagent/main.go:342:	log.G(ctx).WithField("endpts", resp.Endpoints).Info("ConfigureNetworking addrequest")
internal/tools/networkagent/main.go:346:			log.G(ctx).Warn("failed to find endpoint")
internal/tools/networkagent/main.go:350:			log.G(ctx).WithField("name", endpoint.ID).Warn("failed to get endpoint settings")
internal/tools/networkagent/main.go:366:				log.G(ctx).WithField("name", endpoint.ID).Warn("invalid endpoint settings type")
internal/tools/networkagent/main.go:399:			log.G(ctx).Warn("failed to find endpoint to delete")
internal/tools/networkagent/main.go:403:			log.G(ctx).WithField("name", endpoint.ID).Warn("failed to get endpoint settings")
internal/tools/networkagent/main.go:415:				log.G(ctx).WithField("name", endpoint.ID).Warn("invalid endpoint settings type")
internal/tools/networkagent/main.go:420:				log.G(ctx).WithField("name", endpointName).Warn("endpoint was not assigned a NIC ID previously")
internal/tools/networkagent/main.go:430:				log.G(ctx).WithField("name", endpointName).Warn("failed to delete endpoint nic")
internal/tools/networkagent/main.go:439:	log.G(ctx).WithField("req", req).Info("ConfigureNetworking request")
internal/tools/networkagent/main.go:468:		log.G(ctx).WithError(err).Fatalf("failed to read network agent's config file at %s", *configPath)
internal/tools/networkagent/main.go:470:	log.G(ctx).WithFields(logrus.Fields{
internal/tools/networkagent/main.go:488:		log.G(ctx).WithError(err).Fatalf("failed to connect to ncproxy at %s", conf.GRPCAddr)
internal/tools/networkagent/main.go:492:	log.G(ctx).WithField("addr", conf.GRPCAddr).Info("connected to ncproxy")
internal/tools/networkagent/main.go:510:		log.G(ctx).WithError(err).Fatalf("failed to listen on %s", grpcListener.Addr().String())
internal/tools/networkagent/main.go:523:	log.G(ctx).WithField("addr", conf.NodeNetSvcAddr).Info("serving network service agent")
internal/tools/networkagent/main.go:528:		log.G(ctx).Info("Received interrupt. Closing")
internal/tools/networkagent/main.go:531:			log.G(ctx).WithError(err).Fatal("grpc service failure")
internal/tools/networkagent/v0_service_wrapper.go:53:	log.G(ctx).WithField("req", req).Info("ConfigureNetworking request")
internal/tools/uvmboot/lcow.go:292:		entry := log.G(ctx).WithField("flag-value", s)
internal/tools/uvmboot/lcow.go:364:			log.G(ctx).WithError(err).Warn("could not create console from stdin")
internal/tools/uvmboot/mounts.go:34:		log.G(ctx).WithFields(logrus.Fields{
internal/tools/uvmboot/mounts.go:62:		log.G(ctx).WithFields(logrus.Fields{
internal/tools/uvmboot/mounts.go:99:		entry := log.G(ctx).WithField("flag-value", s)
internal/uvm/cimfs.go:38:	log.G(ctx).Tracef("UVMWCOWBlockCIMs : Release")
internal/uvm/cimfs.go:121:					log.G(ctx).WithFields(logrus.Fields{
internal/uvm/cimfs.go:140:		log.G(ctx).WithFields(logrus.Fields{
internal/uvm/computeagent.go:66:	log.G(ctx).WithFields(logrus.Fields{
internal/uvm/computeagent.go:84:	log.G(ctx).WithFields(logrus.Fields{
internal/uvm/computeagent.go:100:	log.G(ctx).WithFields(logrus.Fields{
internal/uvm/computeagent.go:166:	log.G(ctx).WithFields(logrus.Fields{
internal/uvm/computeagent.go:216:	log.G(ctx).WithFields(logrus.Fields{
internal/uvm/computeagent.go:266:	log.G(ctx).WithField("address", l.Addr().String()).Info("serving compute agent")
internal/uvm/computeagent.go:270:			log.G(ctx).WithError(err).Fatal("compute agent: serve failure")
internal/uvm/create.go:282:	log.G(ctx).WithFields(logrus.Fields{
internal/uvm/create.go:316:		log.G(ctx).Errorf("close GCS connection failed: %s", err)
internal/uvm/create.go:337:		e := log.G(ctx).WithField("VMGS file", vmgsFullPath)
internal/uvm/create_lcow.go:159:		log.G(ctx).WithFields(logrus.Fields{
internal/uvm/create_lcow.go:176:		log.G(ctx).WithFields(logrus.Fields{
internal/uvm/create_lcow.go:190:			log.G(ctx).WithFields(logrus.Fields{
internal/uvm/create_lcow.go:542:		log.G(ctx).WithField("options", log.Format(ctx, opts)).Trace("makeLCOWDoc")
internal/uvm/create_lcow.go:630:		log.G(ctx).WithField("resource-partition-id", opts.ResourcePartitionID.String()).Debug("setting resource partition ID")
internal/uvm/create_lcow.go:740:							log.G(ctx).WithError(err).Debug("failed to release memory region")
internal/uvm/create_lcow.go:843:			log.G(ctx).Warn("ignoring `WritableOverlayDirs` option since rootfs is already writable")
internal/uvm/create_lcow.go:897:		log.G(ctx).WithField("options", log.Format(ctx, opts)).Debug("uvm::CreateLCOW options")
internal/uvm/create_lcow.go:943:			log.G(ctx).WithFields(logrus.Fields{
internal/uvm/create_lcow.go:951:			log.G(ctx).WithFields(logrus.Fields{
internal/uvm/create_lcow.go:965:		log.G(ctx).WithField("uvm", log.Format(ctx, uvm)).Trace("create_lcow::CreateLCOW uvm.create result")
internal/uvm/create_lcow.go:986:		log.G(ctx).WithField("vmID", uvm.runtimeID).Debug("Using external GCS bridge")
internal/uvm/create_wcow.go:127:	log.G(ctx).WithFields(logrus.Fields{
internal/uvm/create_wcow.go:292:		log.G(ctx).WithField("resource-partition-id", opts.ResourcePartitionID.String()).Debug("setting resource partition ID")
internal/uvm/create_wcow.go:564:	log.G(ctx).WithField("options", log.Format(ctx, opts)).Debug("uvm::CreateWCOW options")
internal/uvm/create_wcow.go:600:			log.G(ctx).WithFields(logrus.Fields{
internal/uvm/create_wcow.go:608:			log.G(ctx).WithFields(logrus.Fields{
internal/uvm/log_wcow.go:33:		log.G(ctx).WithField("os", uvm.operatingSystem).Error("Log forwarding not supported for this OS")
internal/uvm/modify.go:32:					log.G(ctx).WithError(rerr).Error("failed to roll back resource add")
internal/uvm/modify.go:45:			log.G(ctx).WithError(err).Error("failed to remove host resources after successful guest request")
internal/uvm/network.go:90:			log.G(ctx).Warn(removeErr)
internal/uvm/network.go:100:	l := log.G(ctx).WithField("netns-id", netNS)
internal/uvm/network.go:313:			log.G(ctx).WithFields(logrus.Fields{
internal/uvm/security_policy.go:60:				log.G(ctx).WithField("filePath", fullFilePath).Debug("UVM reference info file not found")
internal/uvm/start.go:66:	log.G(ctx).Debugf("using gcs connection timeout: %s\n", timeout.GCSConnectionTimeout)
internal/uvm/start.go:78:	e := log.G(ctx).WithField(logfields.UVMID, uvm.id)
internal/uvm/stats.go:60:		log.G(ctx).WithFields(logrus.Fields{
internal/uvm/stats.go:78:	log.G(ctx).WithField("vmID", vmIDStr).Debug("looking up vmmem")
internal/uvm/stats.go:91:			log.G(ctx).WithField("pid", pid).Debug("failed to check process")
internal/uvm/stats.go:95:			log.G(ctx).WithField("pid", pid).Debug("found vmmem match")
internal/uvm/vpmem.go:64:			log.G(ctx).WithFields(logrus.Fields{
internal/uvm/vpmem.go:80:			log.G(ctx).WithFields(logrus.Fields{
internal/uvm/vpmem.go:173:	log.G(ctx).WithFields(logrus.Fields{
internal/uvm/vpmem_mapped.go:123:	log.G(ctx).WithFields(logrus.Fields{
internal/uvm/vpmem_mapped.go:138:	log.G(ctx).WithFields(logrus.Fields{
internal/uvm/vpmem_mapped.go:164:	log.G(ctx).WithFields(logrus.Fields{
internal/uvm/vpmem_mapped.go:177:				log.G(ctx).WithFields(logrus.Fields{
internal/uvm/vpmem_mapped.go:213:		log.G(ctx).WithFields(logrus.Fields{
internal/uvm/vpmem_mapped.go:250:				log.G(ctx).WithError(err).Debugf("failed to reclaim pmem region: %s", err)
internal/uvm/vpmem_mapped.go:265:				log.G(ctx).WithError(err).Debugf("failed to rollback modification")
internal/uvm/vpmem_mapped.go:312:		log.G(ctx).WithError(err).Debugf("failed unmapping VHD layer %s", hostPath)
internal/uvm/vsmb.go:186:		log.G(ctx).WithField("path", hostPath).Info("Forcing NoDirectmap for VSMB mount")
internal/uvm/vsmb.go:216:		log.G(ctx).WithFields(logrus.Fields{
internal/uvm/vsmb.go:301:		log.G(ctx).WithFields(logrus.Fields{
internal/uvmfolder/locate.go:35:	log.G(ctx).WithFields(logrus.Fields{
internal/verity/verity.go:41:	log.G(ctx).WithFields(logrus.Fields{
internal/vhdx/info.go:169:			log.G(ctx).WithFields(logrus.Fields{
internal/vhdx/info.go:183:			log.G(ctx).WithFields(logrus.Fields{
internal/vhdx/info.go:226:	log.G(ctx).WithFields(logrus.Fields{
internal/vm/guestmanager/guest.go:40:		log: log.G(ctx).WithField(logfields.UVMID, uvm.ID()),
internal/vm/vmmanager/uvm.go:59:	log.G(ctx).WithFields(logrus.Fields{
internal/vm/vmutils/normalize.go:28:		log.G(ctx).WithFields(logrus.Fields{
internal/vm/vmutils/normalize.go:49:		log.G(ctx).WithFields(logrus.Fields{
internal/vm/vmutils/numa.go:65:			log.G(ctx).WithFields(logrus.Fields{
internal/vm/vmutils/numa.go:72:			log.G(ctx).WithFields(logrus.Fields{
internal/vm/vmutils/numa.go:106:		if entry := log.G(ctx); entry.Logger.IsLevelEnabled(logrus.DebugLevel) {
internal/vm/vmutils/numa.go:138:	if entry := log.G(ctx); entry.Logger.IsLevelEnabled(logrus.DebugLevel) {
internal/vm/vmutils/utils.go:30:			log.G(ctx).WithField("filePath", fullFilePath).Debug("UVM reference info file not found")
internal/vm/vmutils/utils.go:58:	if entry := log.G(ctx); entry.Logger.IsLevelEnabled(logrus.DebugLevel) {
internal/vm/vmutils/vmmem.go:35:			log.G(ctx).WithError(err).Error("failed to create process snapshot")
internal/vm/vmutils/vmmem.go:47:				log.G(ctx).WithError(err).Debug("finished iterating process entries")
internal/vm/vmutils/vmmem.go:62:	log.G(ctx).WithField("vmID", vmIDStr).Debug("looking up vmmem via LookupAccount")
internal/vm/vmutils/vmmem.go:101:			log.G(ctx).WithField("pid", pe32.ProcessID).Debug("found vmmem match")
internal/vmcompute/vmcompute.go:95:		log.G(ctx).WithFields(logrus.Fields{
internal/vmcompute/vmcompute.go:108:			log.G(ctx).WithField(logfields.Timeout, trueTimeout).
internal/wclayer/cim/block_cim_writer.go:64:				log.G(ctx).WithError(err).Warnf("failed to close cim after error: %s", cErr)
internal/wclayer/cim/forked_cim_writer.go:45:				log.G(ctx).WithError(err).Warnf("failed to close cim after error: %s", cErr)
internal/wclayer/cim/forked_cim_writer.go:49:				log.G(ctx).WithError(err).Warnf("failed to cleanup cim after error: %s", cErr)
internal/wclayer/cim/mount.go:69:	log.G(ctx).WithFields(logrus.Fields{
internal/wclayer/getlayermountpath.go:29:	log.G(ctx).Debug("Calling proc (1)")
internal/wclayer/getlayermountpath.go:43:	log.G(ctx).Debug("Calling proc (2)")
internal/windevice/devicequery.go:238:	log.G(ctx).WithFields(logrus.Fields{
internal/windevice/devicequery.go:253:	log.G(ctx).WithField("interface list size", interfaceListSize).Trace("retrieved device interface list size")
internal/windevice/devicequery.go:272:	log.G(ctx).Debugf("disk device interface list: %+v", interfacePaths)
internal/windevice/devicequery.go:293:		log.G(ctx).WithFields(logrus.Fields{
pkg/amdsevsnp/report_windows.go:62:			log.G(ctx).Warnf("Failed to disconnect from service manager: %v", derr)
pkg/amdsevsnp/report_windows.go:96:				log.G(ctx).Tracef("Service %q started successfully", serviceName)
pkg/cimfs/cim_writer_windows.go:365:		log.G(ctx).WithError(err).Warnf("get region files for cim %s", cimPath)
pkg/cimfs/cim_writer_windows.go:372:		log.G(ctx).WithError(err).Warnf("get objectid file for cim %s", cimPath)
pkg/cimfs/cim_writer_windows.go:378:	log.G(ctx).WithFields(logrus.Fields{
pkg/cimfs/cim_writer_windows.go:386:			log.G(ctx).WithError(err).Warnf("remove file %s", regFilePath)
pkg/cimfs/cim_writer_windows.go:395:			log.G(ctx).WithError(err).Warnf("remove file %s", objFilePath)
pkg/cimfs/cim_writer_windows.go:403:		log.G(ctx).WithError(err).Warnf("remove file %s", cimPath)
pkg/cimfs/common.go:108:			log.G(ctx).WithError(err).Warnf("stat for object file %s", path)
pkg/cimfs/common.go:134:			log.G(ctx).WithError(err).Warnf("stat for region file %s", path)
pkg/ociwclayer/cim/import.go:38:	log.G(ctx).WithFields(logrus.Fields{
pkg/ociwclayer/cim/import.go:100:	log.G(ctx).Debugf("writing integrity checksum file for block CIM `%s`", blockPath)
pkg/ociwclayer/cim/import.go:129:	log.G(ctx).WithField("layer", layer).Debug("Importing block CIM layer from tar")
pkg/ociwclayer/cim/import.go:143:	log.G(ctx).WithField("config", *config).Debug("layer import config")
pkg/ociwclayer/cim/import.go:165:		log.G(ctx).Debugf("appending VHD footer to block CIM at `%s`", layer.BlockPath)
pkg/ociwclayer/cim/import.go:284:				log.G(ctx).WithError(flushErr).Warn("flush buffer during layer write failed")
pkg/ociwclayer/cim/import.go:303:	log.G(ctx).WithFields(logrus.Fields{
pkg/ociwclayer/cim/import.go:335:				log.G(ctx).WithError(retErr).Warnf("error in cleanup on failure: %s", rmErr)
pkg/ociwclayer/cim/import.go:361:		log.G(ctx).Debugf("appending VHD footer to block CIM at `%s`", mergedCIM.BlockPath)
pkg/securitypolicy/securitypolicy_options.go:113:	log.G(ctx).WithField("fragment", fmt.Sprintf("%+v", fragment)).Debug("VerifyAndExtractFragment")
pkg/securitypolicy/securitypolicy_options.go:139:	log.G(ctx).WithFields(logrus.Fields{
pkg/securitypolicy/securitypolicy_options.go:146:	log.G(ctx).WithFields(logrus.Fields{
pkg/securitypolicy/securitypolicy_options.go:158:		log.G(ctx).Printf("Badly formed fragment - did resolver failed to match fragment did:x509 from chain with purported issuer %s, feed %s - err %s", issuer, feed, err.Error())
pkg/securitypolicy/securitypolicyenforcer_rego.go:322:		log.G(ctx).WithError(err).Error("unable to marshal error object")
pkg/securitypolicy/securitypolicyenforcer_rego.go:326:	log.G(ctx).WithField("policyDecision", string(decisionJSON))
pkg/securitypolicy/securitypolicyenforcer_rego.go:346:			log.G(ctx).WithError(err).Error("unable to marshal error object")
pkg/securitypolicy/securitypolicyenforcer_rego.go:390:		log.G(ctx).WithError(err).Warn("unable to obtain reason for policy decision")
pkg/securitypolicy/securitypolicyenforcer_rego.go:761:			log.G(ctx).Debugf("Current policy metadata: %s", mdJSON)
pkg/securitypolicy/securitypolicyenforcer_rego.go:763:			log.G(ctx).WithError(err).Warn("failed to obtain policy metadata snapshot")
pkg/securitypolicy/securitypolicyenforcer_rego.go:1161:	log.G(ctx).Tracef("Enforcing verified cims in securitypolicy pkg %+v", layerHashes)
pkg/securitypolicy/securitypolicyenforcer_rego.go:1172:	log.G(ctx).Trace("Enforcing registry changes policy")
pkg/securitypolicy/securitypolicyenforcer_rego.go:1177:		log.G(ctx).Warn("Input registry values are not of expected type")
test/functional/main_test.go:182:		log.G(ctx).WithField("features", flagFeatures.String()).Debug("provided features")
test/functional/main_test.go:220:					log.G(ctx).WithFields(logrus.Fields{
test/functional/main_test.go:368:	e := log.G(ctx).WithFields(logrus.Fields{
test/internal/cmd/cmd.go:43:		Log:                  log.G(ctx),
test/internal/layers/lazy.go:62:	log.G(ctx).WithFields(logrus.Fields{
test/internal/layers/lazy.go:124:	log.G(ctx).WithFields(logrus.Fields{
vendor/github.com/containerd/containerd/v2/core/mount/temp.go:50:			log.G(ctx).WithError(uerr).WithField("dir", root).Error("failed to remove mount temp dir")
vendor/github.com/containerd/containerd/v2/pkg/namespaces/context.go:39:	ctx = context.WithValue(ctx, namespaceKey{}, namespace) // set our key for namespace
vendor/github.com/containerd/containerd/v2/pkg/shim/shim.go:176:	l := log.G(ctx)
vendor/github.com/containerd/containerd/v2/pkg/shim/shim.go:186:	return log.WithLogger(ctx, l), nil
vendor/github.com/containerd/containerd/v2/pkg/shim/shim.go:196:	ctx = log.WithLogger(ctx, log.G(ctx).WithField("runtime", manager.Name()))
vendor/github.com/containerd/containerd/v2/pkg/shim/shim.go:252:	ctx = context.WithValue(ctx, OptsKey{}, Opts{BundlePath: bundlePath, Debug: debugFlag})
vendor/github.com/containerd/containerd/v2/pkg/shim/shim.go:259:		logger := log.G(ctx).WithFields(log.Fields{
vendor/github.com/containerd/containerd/v2/pkg/shim/shim.go:350:		log.G(ctx).WithFields(log.Fields{"id": pID, "type": p.Type}).Debug("loading plugin")
vendor/github.com/containerd/containerd/v2/pkg/shim/shim.go:384:				log.G(ctx).WithFields(log.Fields{"id": pID, "type": p.Type, "error": err}).Info("skip loading plugin")
vendor/github.com/containerd/containerd/v2/pkg/shim/shim.go:391:			log.G(ctx).WithField("id", pID).Debug("registering ttrpc service")
vendor/github.com/containerd/containerd/v2/pkg/shim/shim.go:459:			log.G(ctx).WithError(err).Fatal("containerd-shim: ttrpc server failure")
vendor/github.com/containerd/containerd/v2/pkg/shim/shim.go:465:			log.G(ctx).WithError(err).Warn("Could not setup pprof")
vendor/github.com/containerd/containerd/v2/pkg/shim/shim.go:469:	logger := log.G(ctx).WithFields(log.Fields{
vendor/github.com/containerd/containerd/v2/pkg/shim/shim.go:511:			log.G(ctx).WithError(err).Fatal("containerd-shim: pprof endpoint failure")
vendor/github.com/containerd/continuity/fs/diff.go:104:		log.G(ctx).Debugf("Using single walk diff for %s", b)
vendor/github.com/containerd/continuity/fs/diff.go:108:	log.G(ctx).Debugf("Using double walk diff for %s from %s", b, a)
vendor/github.com/containerd/log/context.go:169:// WithLogger returns a new context with the provided logger. Use in
vendor/github.com/containerd/log/context.go:171:func WithLogger(ctx context.Context, logger *Entry) context.Context {
vendor/github.com/containerd/log/context.go:172:	return context.WithValue(ctx, loggerKey{}, logger.WithContext(ctx))
vendor/github.com/containerd/ttrpc/metadata.go:134:	return context.WithValue(ctx, metadataKey{}, md)
vendor/github.com/containerd/ttrpc/server.go:121:				log.G(ctx).WithError(err).Errorf("ttrpc: failed accept; backoff %v", sleep)
vendor/github.com/containerd/ttrpc/server.go:133:			log.G(ctx).WithError(err).Error("ttrpc: refusing connection after handshake")
vendor/github.com/containerd/ttrpc/server.go:140:			log.G(ctx).WithError(err).Error("ttrpc: create connection failed")
vendor/github.com/containerd/ttrpc/server.go:523:					log.G(ctx).WithError(err).Error("failed marshaling response")
vendor/github.com/containerd/ttrpc/server.go:528:					log.G(ctx).WithError(err).Error("failed sending message on channel")
vendor/github.com/containerd/ttrpc/server.go:540:					log.G(ctx).WithError(err).Error("failed sending message on channel")
vendor/github.com/containerd/ttrpc/server.go:562:			log.G(ctx).WithError(err).Error("error receiving message")
vendor/github.com/go-logr/logr/context_noslog.go:48:	return context.WithValue(ctx, contextKey{}, logger)
vendor/github.com/go-logr/logr/context_slog.go:76:	return context.WithValue(ctx, contextKey{}, logger)
vendor/github.com/go-logr/logr/context_slog.go:82:	return context.WithValue(ctx, contextKey{}, logger)
vendor/github.com/goccy/go-json/internal/encoder/query.go:134:	return context.WithValue(ctx, queryKey{}, query)
vendor/github.com/google/go-containerregistry/internal/redact/redact.go:30:	return context.WithValue(ctx, redactKey, reason)
vendor/github.com/google/go-containerregistry/internal/retry/retry.go:88:	return context.WithValue(ctx, key, true)
vendor/github.com/gorilla/mux/mux.go:449:	ctx := context.WithValue(r.Context(), varsKey, vars)
vendor/github.com/gorilla/mux/mux.go:454:	ctx := context.WithValue(r.Context(), routeKey, route)
vendor/github.com/open-policy-agent/opa/logging/logging.go:237:	return context.WithValue(parent, reqCtxKey, val)
vendor/github.com/open-policy-agent/opa/logging/logging.go:249:	return context.WithValue(parent, httpReqCtxKey, val)
vendor/github.com/open-policy-agent/opa/logging/logging.go:260:	return context.WithValue(parent, decisionCtxKey, id)
vendor/github.com/open-policy-agent/opa/util/decoding/context.go:15:	return context.WithValue(ctx, reqCtxKeyMaxLen, maxLen)
vendor/github.com/open-policy-agent/opa/util/decoding/context.go:19:	return context.WithValue(ctx, reqCtxKeyGzipMaxLen, maxLen)
vendor/go.opencensus.io/plugin/ocgrpc/client_stats_handler.go:48:	return context.WithValue(ctx, rpcDataKey, d)
vendor/go.opencensus.io/plugin/ocgrpc/server_stats_handler.go:45:	return context.WithValue(ctx, rpcDataKey, d)
vendor/go.opencensus.io/tag/context.go:38:	return context.WithValue(ctx, mapCtxKey, m)
vendor/go.opencensus.io/trace/trace.go:123:	return context.WithValue(parent, contextKey{}, s)
vendor/go.opentelemetry.io/otel/internal/baggage/context.go:36:	return context.WithValue(parent, baggageKey, s)
vendor/go.opentelemetry.io/otel/internal/baggage/context.go:50:	return context.WithValue(parent, baggageKey, s)
vendor/go.opentelemetry.io/otel/internal/baggage/context.go:62:	ctx := context.WithValue(parent, baggageKey, s)
vendor/go.opentelemetry.io/otel/trace/context.go:14:	return context.WithValue(parent, currentSpanKey, span)
vendor/golang.org/x/net/http2/server.go:570:	ctx = context.WithValue(ctx, http.LocalAddrContextKey, c.LocalAddr())
vendor/golang.org/x/net/http2/server.go:572:		ctx = context.WithValue(ctx, http.ServerContextKey, hs)
vendor/golang.org/x/net/trace/trace.go:137:	return context.WithValue(ctx, contextKey, tr)
vendor/google.golang.org/grpc/credentials/credentials.go:261:	return context.WithValue(ctx, requestInfoKey{}, ri)
vendor/google.golang.org/grpc/internal/credentials/credentials.go:34:	return context.WithValue(ctx, clientHandshakeInfoKey{}, chi)
vendor/google.golang.org/grpc/internal/grpcutil/metadata.go:31:	return context.WithValue(ctx, mdExtraKey{}, md)
vendor/google.golang.org/grpc/internal/stats/labels.go:41:	return context.WithValue(ctx, labelsKey{}, labels)
vendor/google.golang.org/grpc/internal/transport/http2_server.go:1497:	return context.WithValue(ctx, connectionKey{}, conn)
vendor/google.golang.org/grpc/metadata/metadata.go:165:	return context.WithValue(ctx, mdIncomingKey{}, md)
vendor/google.golang.org/grpc/metadata/metadata.go:173:	return context.WithValue(ctx, mdOutgoingKey{}, rawMD{md: md})
vendor/google.golang.org/grpc/metadata/metadata.go:191:	return context.WithValue(ctx, mdOutgoingKey{}, rawMD{md: md.md, added: added})
vendor/google.golang.org/grpc/peer/peer.go:76:	return context.WithValue(ctx, peerKey{}, p)
vendor/google.golang.org/grpc/rpc_util.go:1050:	return context.WithValue(ctx, rpcInfoContextKey{}, &rpcInfo{
vendor/google.golang.org/grpc/server.go:1901:	return context.WithValue(ctx, streamKey{}, stream)
vendor/google.golang.org/grpc/server.go:2051:	return context.WithValue(ctx, serverKey{}, server)
```

## Public API exposure

```txt
cmd/gcs/main.go:37:func memoryLogFormat(metrics *cgroupstats.Metrics) logrus.Fields {
internal/cmd/io.go:67:func relayIO(w io.Writer, r io.Reader, log *logrus.Entry, name string) (int64, error) {
internal/gcs/bridge.go:75:func newBridge(conn io.ReadWriteCloser, notify notifyFunc, log *logrus.Entry) *bridge {
internal/guest/kmsg/kmsg.go:62:func (ke *Entry) logFormat() logrus.Fields {
internal/log/context.go:39:func GetEntry(ctx context.Context) *logrus.Entry {
internal/log/context.go:56:func SetEntry(ctx context.Context, fields logrus.Fields) (context.Context, *logrus.Entry) {
internal/log/context.go:86:func WithContext(ctx context.Context, entry *logrus.Entry) (context.Context, *logrus.Entry) {
internal/log/context.go:95:func fromContext(ctx context.Context) *logrus.Entry {
internal/log/hook.go:57:func (h *Hook) Levels() []logrus.Level {
internal/log/hook.go:61:func (h *Hook) Fire(e *logrus.Entry) (err error) {
internal/log/hook.go:80:func (h *Hook) encode(e *logrus.Entry) {
internal/log/hook.go:161:func (h *Hook) addSpanContext(e *logrus.Entry) {
internal/log/nopformatter.go:12:func (NopFormatter) Format(*logrus.Entry) ([]byte, error) { return nil, nil }
internal/vm/vmutils/gcs_logs_test.go:397:func (h *testLogHook) Levels() []logrus.Level {
internal/vm/vmutils/gcs_logs_test.go:401:func (h *testLogHook) Fire(entry *logrus.Entry) error {
vendor/github.com/Microsoft/go-winio/pkg/etwlogrus/hook.go:75:func (*Hook) Levels() []logrus.Level {
vendor/github.com/Microsoft/go-winio/pkg/etwlogrus/hook.go:90:func (h *Hook) Fire(e *logrus.Entry) error {
vendor/github.com/Microsoft/go-winio/pkg/etwlogrus/opts.go:40:func WithGetName(f func(*logrus.Entry) string) HookOpt {
vendor/github.com/Microsoft/go-winio/pkg/etwlogrus/opts.go:48:func WithEventOpts(f func(*logrus.Entry) []etw.EventOpt) HookOpt {
vendor/github.com/containerd/log/context.go:71:type Entry = logrus.Entry
vendor/github.com/containerd/log/context.go:79:type Level = logrus.Level
vendor/github.com/open-policy-agent/opa/logging/logging.go:66:func (l *StandardLogger) SetFormatter(formatter logrus.Formatter) {
vendor/github.com/open-policy-agent/opa/logging/logging.go:226:func (rctx RequestContext) Fields() logrus.Fields {
vendor/github.com/opencontainers/runc/libcontainer/logs/logs.go:41:func processEntry(text []byte, logger *logrus.Logger) {
```

## Potential hot-path logging

```txt
cmd/containerd-shim-lcow-v2/manager.go:211:		logrus.WithField("log", string(logBytes)).Warn("found shim panic logs during delete")
cmd/containerd-shim-lcow-v2/manager.go:213:		logrus.WithError(err).Warn("failed to open shim panic log")
cmd/containerd-shim-lcow-v2/manager.go:272:func (m *shimManager) Info(_ context.Context, optionsR io.Reader) (*types.RuntimeInfo, error) {
cmd/containerd-shim-lcow-v2/service/plugin/plugin.go:37:		logrus.Error(err)
cmd/containerd-shim-lcow-v2/service/plugin/plugin.go:42:			logrus.Error(err)
cmd/containerd-shim-lcow-v2/service/plugin/plugin.go:102:			logrus.Warn("service not initialized")
cmd/containerd-shim-lcow-v2/service/plugin/plugin.go:110:		log.WithField("stack", resp.Stacks).Info("goroutine stack dump")
cmd/containerd-shim-lcow-v2/service/plugin/plugin.go:112:			log.WithField("stack", resp.GuestStacks).Info("guest stack dump")
cmd/containerd-shim-lcow-v2/service/service.go:116:			log.G(ctx).WithError(err).Error("post event")
cmd/containerd-shim-lcow-v2/service/service_sandbox_internal.go:112:		uvmReferenceInfoEncoded, err := vmutils.ParseUVMReferenceInfo(
cmd/containerd-shim-lcow-v2/service/service_sandbox_internal.go:276:			log.G(ctx).WithError(err).Error("failed to terminate VM during shutdown")
cmd/containerd-shim-runhcs-v1/delete.go:80:			logrus.WithField("log", string(logBytes)).Warn("found shim panic logs during delete")
cmd/containerd-shim-runhcs-v1/delete.go:82:			logrus.WithError(err).Warn("failed to open shim panic log")
cmd/containerd-shim-runhcs-v1/exec.go:88:func newExecInvalidStateError(tid, eid string, state shimExecState, op string) error {
cmd/containerd-shim-runhcs-v1/exec_hcs.go:181:		return newExecInvalidStateError(he.tid, he.id, he.state, "start")
cmd/containerd-shim-runhcs-v1/exec_hcs.go:340:		return newExecInvalidStateError(he.tid, he.id, he.state, "kill")
cmd/containerd-shim-runhcs-v1/exec_hcs.go:409:		log.G(ctx).WithField("status", status).Debug("hcsExec::exitFromCreatedL")
cmd/containerd-shim-runhcs-v1/exec_hcs.go:460:		log.G(ctx).WithError(err).Error("failed process Wait")
cmd/containerd-shim-runhcs-v1/exec_hcs.go:469:		log.G(ctx).WithError(err).Error("failed to get ExitCode")
cmd/containerd-shim-runhcs-v1/exec_hcs.go:471:		log.G(ctx).WithField("exitCode", code).Debug("exited")
cmd/containerd-shim-runhcs-v1/exec_hcs.go:498:			log.G(ctx).WithError(err).Error("failed to publish TaskExitEvent")
cmd/containerd-shim-runhcs-v1/exec_wcow_podsandbox.go:26:	}).Debug("newWcowPodSandboxExec")
cmd/containerd-shim-runhcs-v1/exec_wcow_podsandbox.go:130:		return newExecInvalidStateError(wpse.tid, wpse.tid, wpse.state, "start")
cmd/containerd-shim-runhcs-v1/exec_wcow_podsandbox.go:171:		return newExecInvalidStateError(wpse.tid, wpse.tid, wpse.state, "kill")
cmd/containerd-shim-runhcs-v1/exec_wcow_podsandbox.go:197:		log.G(ctx).WithField("status", status).Debug("wcowPodSandboxExec::ForceExit")
cmd/containerd-shim-runhcs-v1/exec_wcow_podsandbox_test.go:180:	verifyExpectedError(t, nil, err, errdefs.ErrFailedPrecondition)
cmd/containerd-shim-runhcs-v1/exec_wcow_podsandbox_test.go:232:	verifyExpectedError(t, nil, err, errdefs.ErrFailedPrecondition)
cmd/containerd-shim-runhcs-v1/exec_wcow_podsandbox_test.go:241:	verifyExpectedError(t, nil, err, errdefs.ErrFailedPrecondition)
cmd/containerd-shim-runhcs-v1/exec_wcow_podsandbox_test.go:250:	verifyExpectedError(t, nil, err, errdefs.ErrFailedPrecondition)
cmd/containerd-shim-runhcs-v1/main.go:64:		log.WithField("stack", resp.Stacks).Info("goroutine stack dump")
cmd/containerd-shim-runhcs-v1/main.go:66:			log.WithField("stack", resp.GuestStacks).Info("guest stack dump")
cmd/containerd-shim-runhcs-v1/main.go:78:		logrus.Error(err)
cmd/containerd-shim-runhcs-v1/main.go:83:			logrus.Error(err)
cmd/containerd-shim-runhcs-v1/options/runhcs.pb.go:200:	ms.StoreMessageInfo(mi)
cmd/containerd-shim-runhcs-v1/options/runhcs.pb.go:213:		if ms.LoadMessageInfo() == nil {
cmd/containerd-shim-runhcs-v1/options/runhcs.pb.go:214:			ms.StoreMessageInfo(mi)
cmd/containerd-shim-runhcs-v1/options/runhcs.pb.go:226:func (x *Options) GetDebug() bool {
cmd/containerd-shim-runhcs-v1/options/runhcs.pb.go:387:	ms.StoreMessageInfo(mi)
cmd/containerd-shim-runhcs-v1/options/runhcs.pb.go:400:		if ms.LoadMessageInfo() == nil {
cmd/containerd-shim-runhcs-v1/options/runhcs.pb.go:401:			ms.StoreMessageInfo(mi)
cmd/containerd-shim-runhcs-v1/pod.go:40:	log.G(ctx).WithField("options", log.Format(ctx, *wopts)).Debug("initialize WCOW boot files")
cmd/containerd-shim-runhcs-v1/pod.go:136:	log.G(ctx).WithField("tid", req.ID).Debug("createPod")
cmd/containerd-shim-runhcs-v1/pod_test.go:138:func Test_pod_GetTask_WorkloadID_NotCreated_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/pod_test.go:142:	verifyExpectedError(t, t1, err, errdefs.ErrNotFound)
cmd/containerd-shim-runhcs-v1/pod_test.go:160:func Test_pod_KillTask_UnknownTaskID_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/pod_test.go:164:	verifyExpectedError(t, nil, err, errdefs.ErrNotFound)
cmd/containerd-shim-runhcs-v1/pod_test.go:167:func Test_pod_KillTask_SandboxID_UnknownExecID_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/pod_test.go:171:	verifyExpectedError(t, nil, err, errdefs.ErrNotFound)
cmd/containerd-shim-runhcs-v1/pod_test.go:203:func Test_pod_KillTask_SandboxID_2ndExecID_All_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/pod_test.go:208:		verifyExpectedError(t, nil, err, errdefs.ErrFailedPrecondition)
cmd/containerd-shim-runhcs-v1/pod_test.go:244:func Test_pod_KillTask_WorkloadID_2ndExecID_All_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/pod_test.go:251:		verifyExpectedError(t, nil, err, errdefs.ErrFailedPrecondition)
cmd/containerd-shim-runhcs-v1/pod_test.go:291:	verifyExpectedError(t, nil, err, errdefs.ErrFailedPrecondition)
cmd/containerd-shim-runhcs-v1/pod_test.go:328:	verifyExpectedError(t, nil, err, errdefs.ErrNotFound)
cmd/containerd-shim-runhcs-v1/pod_test.go:346:	verifyExpectedError(t, nil, err, errdefs.ErrFailedPrecondition)
cmd/containerd-shim-runhcs-v1/pod_test.go:379:	verifyExpectedError(t, nil, err, errdefs.ErrNotFound)
cmd/containerd-shim-runhcs-v1/pod_test.go:389:	verifyExpectedError(t, nil, err, errdefs.ErrNotFound)
cmd/containerd-shim-runhcs-v1/serve.go:219:				logrus.WithError(err).Fatal("containerd-shim: ttrpc server failure")
cmd/containerd-shim-runhcs-v1/serve.go:267:	if err == nil || strings.Contains(err.Error(), "use of closed network connection") {
cmd/containerd-shim-runhcs-v1/serve.go:324:	logrus.WithField("event", event).Info("Halting until signalled")
cmd/containerd-shim-runhcs-v1/service.go:535:func (s *service) ComputeProcessorInfo(ctx context.Context, req *extendedtask.ComputeProcessorInfoRequest) (*extendedtask.ComputeProcessorInfoResponse, error) {
cmd/containerd-shim-runhcs-v1/service_internal.go:90:			entry.WithField("options", log.Format(ctx, shimOpts)).Debug("parsed runhcs runtime options")
cmd/containerd-shim-runhcs-v1/service_internal.go:548:	info, err := t.ProcessorInfo(ctx)
cmd/containerd-shim-runhcs-v1/service_internal_podshim_test.go:68:func Test_PodShim_getPod_NotCreated_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/service_internal_podshim_test.go:76:	verifyExpectedError(t, p, err, errdefs.ErrFailedPrecondition)
cmd/containerd-shim-runhcs-v1/service_internal_podshim_test.go:91:func Test_PodShim_getTask_NotCreated_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/service_internal_podshim_test.go:99:	verifyExpectedError(t, st, err, errdefs.ErrNotFound)
cmd/containerd-shim-runhcs-v1/service_internal_podshim_test.go:102:func Test_PodShim_getTask_Created_DifferentID_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/service_internal_podshim_test.go:107:	verifyExpectedError(t, st, err, errdefs.ErrNotFound)
cmd/containerd-shim-runhcs-v1/service_internal_podshim_test.go:134:func Test_PodShim_stateInternal_NoTask_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/service_internal_podshim_test.go:142:	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
cmd/containerd-shim-runhcs-v1/service_internal_podshim_test.go:145:func Test_PodShim_stateInternal_InitTaskID_DifferentExecID_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/service_internal_podshim_test.go:153:	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
cmd/containerd-shim-runhcs-v1/service_internal_podshim_test.go:206:func Test_PodShim_startInternal_NoTask_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/service_internal_podshim_test.go:214:	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
cmd/containerd-shim-runhcs-v1/service_internal_podshim_test.go:217:func Test_PodShim_startInternal_ValidTask_DifferentExecID_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/service_internal_podshim_test.go:225:	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
cmd/containerd-shim-runhcs-v1/service_internal_podshim_test.go:264:func Test_PodShim_deleteInternal_NoTask_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/service_internal_podshim_test.go:272:	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
cmd/containerd-shim-runhcs-v1/service_internal_podshim_test.go:275:func Test_PodShim_deleteInternal_ValidTask_DifferentExecID_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/service_internal_podshim_test.go:283:	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
cmd/containerd-shim-runhcs-v1/service_internal_podshim_test.go:328:func Test_PodShim_pidsInternal_NoTask_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/service_internal_podshim_test.go:336:	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
cmd/containerd-shim-runhcs-v1/service_internal_podshim_test.go:403:func Test_PodShim_pauseInternal_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/service_internal_podshim_test.go:411:	verifyExpectedError(t, resp, err, errdefs.ErrNotImplemented)
cmd/containerd-shim-runhcs-v1/service_internal_podshim_test.go:414:func Test_PodShim_resumeInternal_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/service_internal_podshim_test.go:422:	verifyExpectedError(t, resp, err, errdefs.ErrNotImplemented)
cmd/containerd-shim-runhcs-v1/service_internal_podshim_test.go:425:func Test_PodShim_checkpointInternal_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/service_internal_podshim_test.go:433:	verifyExpectedError(t, resp, err, errdefs.ErrNotImplemented)
cmd/containerd-shim-runhcs-v1/service_internal_podshim_test.go:436:func Test_PodShim_killInternal_NoTask_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/service_internal_podshim_test.go:444:	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
cmd/containerd-shim-runhcs-v1/service_internal_podshim_test.go:447:func Test_PodShim_killInternal_InitTaskID_DifferentExecID_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/service_internal_podshim_test.go:455:	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
cmd/containerd-shim-runhcs-v1/service_internal_podshim_test.go:490:func Test_PodShim_resizePtyInternal_NoTask_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/service_internal_podshim_test.go:498:	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
cmd/containerd-shim-runhcs-v1/service_internal_podshim_test.go:501:func Test_PodShim_resizePtyInternal_InitTaskID_DifferentExecID_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/service_internal_podshim_test.go:509:	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
cmd/containerd-shim-runhcs-v1/service_internal_podshim_test.go:542:func Test_PodShim_closeIOInternal_NoTask_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/service_internal_podshim_test.go:550:	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
cmd/containerd-shim-runhcs-v1/service_internal_podshim_test.go:553:func Test_PodShim_closeIOInternal_InitTaskID_DifferentExecID_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/service_internal_podshim_test.go:561:	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
cmd/containerd-shim-runhcs-v1/service_internal_podshim_test.go:623:func Test_PodShim_updateInternal_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/service_internal_podshim_test.go:647:func Test_PodShim_waitInternal_NoTask_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/service_internal_podshim_test.go:655:	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
cmd/containerd-shim-runhcs-v1/service_internal_podshim_test.go:658:func Test_PodShim_waitInternal_InitTaskID_DifferentExecID_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/service_internal_podshim_test.go:666:	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
cmd/containerd-shim-runhcs-v1/service_internal_taskshim_test.go:57:func Test_TaskShim_getTask_NotCreated_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/service_internal_taskshim_test.go:65:	verifyExpectedError(t, st, err, errdefs.ErrNotFound)
cmd/containerd-shim-runhcs-v1/service_internal_taskshim_test.go:68:func Test_TaskShim_getTask_Created_DifferentID_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/service_internal_taskshim_test.go:73:	verifyExpectedError(t, st, err, errdefs.ErrNotFound)
cmd/containerd-shim-runhcs-v1/service_internal_taskshim_test.go:88:func Test_TaskShim_stateInternal_NoTask_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/service_internal_taskshim_test.go:96:	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
cmd/containerd-shim-runhcs-v1/service_internal_taskshim_test.go:99:func Test_TaskShim_stateInternal_InitTaskID_DifferentExecID_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/service_internal_taskshim_test.go:107:	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
cmd/containerd-shim-runhcs-v1/service_internal_taskshim_test.go:160:func Test_TaskShim_startInternal_NoTask_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/service_internal_taskshim_test.go:168:	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
cmd/containerd-shim-runhcs-v1/service_internal_taskshim_test.go:171:func Test_TaskShim_startInternal_ValidTask_DifferentExecID_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/service_internal_taskshim_test.go:179:	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
cmd/containerd-shim-runhcs-v1/service_internal_taskshim_test.go:218:func Test_TaskShim_deleteInternal_NoTask_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/service_internal_taskshim_test.go:226:	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
cmd/containerd-shim-runhcs-v1/service_internal_taskshim_test.go:229:func Test_TaskShim_deleteInternal_ValidTask_DifferentExecID_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/service_internal_taskshim_test.go:237:	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
cmd/containerd-shim-runhcs-v1/service_internal_taskshim_test.go:282:func Test_TaskShim_pidsInternal_NoTask_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/service_internal_taskshim_test.go:290:	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
cmd/containerd-shim-runhcs-v1/service_internal_taskshim_test.go:328:func Test_TaskShim_pauseInternal_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/service_internal_taskshim_test.go:336:	verifyExpectedError(t, resp, err, errdefs.ErrNotImplemented)
cmd/containerd-shim-runhcs-v1/service_internal_taskshim_test.go:339:func Test_TaskShim_resumeInternal_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/service_internal_taskshim_test.go:347:	verifyExpectedError(t, resp, err, errdefs.ErrNotImplemented)
cmd/containerd-shim-runhcs-v1/service_internal_taskshim_test.go:350:func Test_TaskShim_checkpointInternal_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/service_internal_taskshim_test.go:358:	verifyExpectedError(t, resp, err, errdefs.ErrNotImplemented)
cmd/containerd-shim-runhcs-v1/service_internal_taskshim_test.go:361:func Test_TaskShim_killInternal_NoTask_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/service_internal_taskshim_test.go:369:	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
cmd/containerd-shim-runhcs-v1/service_internal_taskshim_test.go:372:func Test_TaskShim_killInternal_InitTaskID_DifferentExecID_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/service_internal_taskshim_test.go:380:	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
cmd/containerd-shim-runhcs-v1/service_internal_taskshim_test.go:415:func Test_TaskShim_resizePtyInternal_NoTask_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/service_internal_taskshim_test.go:423:	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
cmd/containerd-shim-runhcs-v1/service_internal_taskshim_test.go:426:func Test_TaskShim_resizePtyInternal_InitTaskID_DifferentExecID_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/service_internal_taskshim_test.go:434:	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
cmd/containerd-shim-runhcs-v1/service_internal_taskshim_test.go:467:func Test_TaskShim_closeIOInternal_NoTask_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/service_internal_taskshim_test.go:475:	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
cmd/containerd-shim-runhcs-v1/service_internal_taskshim_test.go:478:func Test_TaskShim_closeIOInternal_InitTaskID_DifferentExecID_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/service_internal_taskshim_test.go:486:	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
cmd/containerd-shim-runhcs-v1/service_internal_taskshim_test.go:585:func Test_TaskShimWindowsMount_updateInternal_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/service_internal_taskshim_test.go:620:func Test_TaskShim_updateInternal_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/service_internal_taskshim_test.go:642:func Test_TaskShim_waitInternal_NoTask_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/service_internal_taskshim_test.go:650:	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
cmd/containerd-shim-runhcs-v1/service_internal_taskshim_test.go:653:func Test_TaskShim_waitInternal_InitTaskID_DifferentExecID_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/service_internal_taskshim_test.go:661:	verifyExpectedError(t, resp, err, errdefs.ErrNotFound)
cmd/containerd-shim-runhcs-v1/service_internal_test.go:18:func verifyExpectedError(t *testing.T, resp interface{}, actual, expected error) {
cmd/containerd-shim-runhcs-v1/stats/stats.pb.go:42:	ms.StoreMessageInfo(mi)
cmd/containerd-shim-runhcs-v1/stats/stats.pb.go:55:		if ms.LoadMessageInfo() == nil {
cmd/containerd-shim-runhcs-v1/stats/stats.pb.go:56:			ms.StoreMessageInfo(mi)
cmd/containerd-shim-runhcs-v1/stats/stats.pb.go:132:	ms.StoreMessageInfo(mi)
cmd/containerd-shim-runhcs-v1/stats/stats.pb.go:145:		if ms.LoadMessageInfo() == nil {
cmd/containerd-shim-runhcs-v1/stats/stats.pb.go:146:			ms.StoreMessageInfo(mi)
cmd/containerd-shim-runhcs-v1/stats/stats.pb.go:213:	ms.StoreMessageInfo(mi)
cmd/containerd-shim-runhcs-v1/stats/stats.pb.go:226:		if ms.LoadMessageInfo() == nil {
cmd/containerd-shim-runhcs-v1/stats/stats.pb.go:227:			ms.StoreMessageInfo(mi)
cmd/containerd-shim-runhcs-v1/stats/stats.pb.go:273:	ms.StoreMessageInfo(mi)
cmd/containerd-shim-runhcs-v1/stats/stats.pb.go:286:		if ms.LoadMessageInfo() == nil {
cmd/containerd-shim-runhcs-v1/stats/stats.pb.go:287:			ms.StoreMessageInfo(mi)
cmd/containerd-shim-runhcs-v1/stats/stats.pb.go:334:	ms.StoreMessageInfo(mi)
cmd/containerd-shim-runhcs-v1/stats/stats.pb.go:347:		if ms.LoadMessageInfo() == nil {
cmd/containerd-shim-runhcs-v1/stats/stats.pb.go:348:			ms.StoreMessageInfo(mi)
cmd/containerd-shim-runhcs-v1/stats/stats.pb.go:400:	ms.StoreMessageInfo(mi)
cmd/containerd-shim-runhcs-v1/stats/stats.pb.go:413:		if ms.LoadMessageInfo() == nil {
cmd/containerd-shim-runhcs-v1/stats/stats.pb.go:414:			ms.StoreMessageInfo(mi)
cmd/containerd-shim-runhcs-v1/stats/stats.pb.go:451:	ms.StoreMessageInfo(mi)
cmd/containerd-shim-runhcs-v1/stats/stats.pb.go:464:		if ms.LoadMessageInfo() == nil {
cmd/containerd-shim-runhcs-v1/stats/stats.pb.go:465:			ms.StoreMessageInfo(mi)
cmd/containerd-shim-runhcs-v1/stats/stats.pb.go:497:	ms.StoreMessageInfo(mi)
cmd/containerd-shim-runhcs-v1/stats/stats.pb.go:510:		if ms.LoadMessageInfo() == nil {
cmd/containerd-shim-runhcs-v1/stats/stats.pb.go:511:			ms.StoreMessageInfo(mi)
cmd/containerd-shim-runhcs-v1/stats/stats.pb.go:561:	ms.StoreMessageInfo(mi)
cmd/containerd-shim-runhcs-v1/stats/stats.pb.go:574:		if ms.LoadMessageInfo() == nil {
cmd/containerd-shim-runhcs-v1/stats/stats.pb.go:575:			ms.StoreMessageInfo(mi)
cmd/containerd-shim-runhcs-v1/task.go:99:	ProcessorInfo(ctx context.Context) (*processorInfo, error)
cmd/containerd-shim-runhcs-v1/task_hcs.go:56:	log.G(ctx).WithField("tid", req.ID).Debug("newHcsStandaloneTask")
cmd/containerd-shim-runhcs-v1/task_hcs.go:194:	}).Debug("newHcsTask")
cmd/containerd-shim-runhcs-v1/task_hcs.go:439:				}).Warn("failed to kill exec in task")
cmd/containerd-shim-runhcs-v1/task_hcs.go:494:		return 0, 0, time.Time{}, newExecInvalidStateError(ht.id, eid, state, "delete")
cmd/containerd-shim-runhcs-v1/task_hcs.go:511:			log.G(ctx).Error("timed out waiting for resource cleanup")
cmd/containerd-shim-runhcs-v1/task_hcs.go:533:				log.G(ctx).WithError(err).Errorf("failed to delete container state")
cmd/containerd-shim-runhcs-v1/task_hcs.go:634:		log.G(ctx).WithError(err).Error("failed to wait for host virtual machine exit")
cmd/containerd-shim-runhcs-v1/task_hcs.go:636:		log.G(ctx).Debug("host virtual machine exited")
cmd/containerd-shim-runhcs-v1/task_hcs.go:658:		log.G(ctx).Debug("hcsTask::closeOnce")
cmd/containerd-shim-runhcs-v1/task_hcs.go:675:				log.G(ctx).WithError(err).Error("failed to shutdown container")
cmd/containerd-shim-runhcs-v1/task_hcs.go:683:						log.G(ctx).WithError(err).Error("failed to wait for container shutdown")
cmd/containerd-shim-runhcs-v1/task_hcs.go:687:					log.G(ctx).WithError(err).Error("failed to wait for container shutdown")
cmd/containerd-shim-runhcs-v1/task_hcs.go:694:					log.G(ctx).WithError(err).Error("failed to terminate container")
cmd/containerd-shim-runhcs-v1/task_hcs.go:702:							log.G(ctx).WithError(err).Error("failed to wait for container terminate")
cmd/containerd-shim-runhcs-v1/task_hcs.go:705:						log.G(ctx).WithError(hcs.ErrTimeout).Error("failed to wait for container terminate")
cmd/containerd-shim-runhcs-v1/task_hcs.go:712:				log.G(ctx).WithError(err).Error("failed to release container resources")
cmd/containerd-shim-runhcs-v1/task_hcs.go:717:				log.G(ctx).WithError(err).Error("failed to close container")
cmd/containerd-shim-runhcs-v1/task_hcs.go:733:		log.G(ctx).Debug("hcsTask::closeHostOnce")
cmd/containerd-shim-runhcs-v1/task_hcs.go:737:				log.G(ctx).WithError(err).Error("failed host vm shutdown")
cmd/containerd-shim-runhcs-v1/task_hcs.go:753:			log.G(ctx).WithError(err).Error("failed to publish TaskExitEventTopic")
cmd/containerd-shim-runhcs-v1/task_hcs.go:781:			log.G(ctx).WithError(err).Warn("failed to capture guest stacks")
cmd/containerd-shim-runhcs-v1/task_hcs.go:966:func (ht *hcsTask) ProcessorInfo(ctx context.Context) (*processorInfo, error) {
cmd/containerd-shim-runhcs-v1/task_hcs_test.go:50:func Test_hcsTask_GetExec_UnknownExecID_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/task_hcs_test.go:55:	verifyExpectedError(t, e, err, errdefs.ErrNotFound)
cmd/containerd-shim-runhcs-v1/task_hcs_test.go:70:func Test_hcsTask_KillExec_UnknownExecID_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/task_hcs_test.go:75:	verifyExpectedError(t, nil, err, errdefs.ErrNotFound)
cmd/containerd-shim-runhcs-v1/task_hcs_test.go:122:func Test_hcsTask_KillExec_2ndExecID_All_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/task_hcs_test.go:127:	verifyExpectedError(t, nil, err, errdefs.ErrFailedPrecondition)
cmd/containerd-shim-runhcs-v1/task_hcs_test.go:156:func Test_hcsTask_DeleteExec_UnknownExecID_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/task_hcs_test.go:160:	verifyExpectedError(t, nil, err, errdefs.ErrNotFound)
cmd/containerd-shim-runhcs-v1/task_hcs_test.go:180:func Test_hcsTask_DeleteExec_InitExecID_RunningState_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/task_hcs_test.go:193:	verifyExpectedError(t, nil, err, errdefs.ErrFailedPrecondition)
cmd/containerd-shim-runhcs-v1/task_hcs_test.go:215:func Test_hcsTask_DeleteExec_InitExecID_2ndExec_CreatedState_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/task_hcs_test.go:226:	verifyExpectedError(t, nil, err, errdefs.ErrFailedPrecondition)
cmd/containerd-shim-runhcs-v1/task_hcs_test.go:233:func Test_hcsTask_DeleteExec_InitExecID_2ndExec_RunningState_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/task_hcs_test.go:247:	verifyExpectedError(t, nil, err, errdefs.ErrFailedPrecondition)
cmd/containerd-shim-runhcs-v1/task_hcs_test.go:288:func Test_hcsTask_DeleteExec_2ndExecID_RunningState_Error(t *testing.T) {
cmd/containerd-shim-runhcs-v1/task_hcs_test.go:300:	verifyExpectedError(t, nil, err, errdefs.ErrFailedPrecondition)
cmd/containerd-shim-runhcs-v1/task_test.go:135:func (tst *testShimTask) ProcessorInfo(ctx context.Context) (*processorInfo, error) {
cmd/containerd-shim-runhcs-v1/task_wcow_podsandbox.go:39:	log.G(ctx).WithField("tid", id).Debug("newWcowPodSandboxTask")
cmd/containerd-shim-runhcs-v1/task_wcow_podsandbox.go:144:		return 0, 0, time.Time{}, newExecInvalidStateError(wpst.id, eid, state, "delete")
cmd/containerd-shim-runhcs-v1/task_wcow_podsandbox.go:187:		log.G(ctx).Debug("wcowPodSandboxTask::closeOnce")
cmd/containerd-shim-runhcs-v1/task_wcow_podsandbox.go:191:				log.G(ctx).WithError(err).Error("failed to cleanup networking for utility VM")
cmd/containerd-shim-runhcs-v1/task_wcow_podsandbox.go:195:				log.G(ctx).WithError(err).Error("failed host vm shutdown")
cmd/containerd-shim-runhcs-v1/task_wcow_podsandbox.go:211:			log.G(ctx).WithError(err).Error("failed to publish TaskExitEventTopic")
cmd/containerd-shim-runhcs-v1/task_wcow_podsandbox.go:236:		log.G(ctx).WithError(werr).Error("parent wait failed")
cmd/containerd-shim-runhcs-v1/task_wcow_podsandbox.go:268:			log.G(ctx).WithError(err).Warn("failed to capture guest stacks")
cmd/containerd-shim-runhcs-v1/task_wcow_podsandbox.go:313:func (wpst *wcowPodSandboxTask) ProcessorInfo(ctx context.Context) (*processorInfo, error) {
cmd/gcs-sidecar/main.go:160:		logrus.WithError(err).Error("error redirecting handle")
cmd/gcs-sidecar/main.go:178:		logrus.Error("context deadline exceeded")
cmd/gcs-sidecar/main.go:182:			logrus.Error(r)
cmd/gcs-sidecar/main.go:192:		logrus.WithError(vsmbError).Errorf("VSMB redirector initialization failed.")
cmd/gcs-sidecar/main.go:201:		logrus.WithError(err).Error("error starting listener for sidecar <-> inbox gcs communication")
cmd/gcs-sidecar/main.go:208:		logrus.WithError(err).Error("error accepting inbox GCS connection")
cmd/gcs-sidecar/main.go:223:		logrus.WithError(err).Error("error dialing hcsshim external bridge")
cmd/gcs-sidecar/main.go:228:		// When error happens, pspdriver.GetPspDriverError() returns true.
cmd/gcs-sidecar/main.go:231:		logrus.WithError(err).Errorf("failed to start PSP driver")
cmd/gcs-sidecar/main.go:245:		logrus.WithError(err).Error("failed to serve request")
cmd/gcs/main.go:58:			logrus.WithError(err).WithField("cgroup", cgName).Error("failed to read from eventfd")
cmd/gcs/main.go:92:			entry.WithError(err).Error(msg)
cmd/gcs/main.go:94:			entry.WithFields(memoryLogFormat(metrics)).Warn(msg)
cmd/gcs/main.go:114:			}).Warn("restart monitor: run command returns error")
cmd/gcs/main.go:285:	}).Info("GCS started")
cmd/gcs/main.go:296:			logrus.WithError(err).Fatal("failed to set core dump location")
cmd/gcs/main.go:312:		logrus.WithError(err).Fatal("failed to enable hierarchy support for root cgroup")
cmd/gcs/main.go:322:		logrus.WithError(err).Fatal("failed to get sys info")
cmd/gcs/main.go:331:		logrus.WithError(err).Fatal("failed to create containers cgroup")
cmd/gcs/main.go:343:		logrus.WithError(err).Fatal("failed to create containers/virtual-pods cgroup")
cmd/gcs/main.go:349:		logrus.WithError(err).Fatal("failed to create gcs cgroup")
cmd/gcs/main.go:353:		logrus.WithError(err).Fatal("failed add gcs pid to gcs cgroup")
cmd/gcs/main.go:359:		logrus.WithError(err).Fatal("failed to initialize new runc runtime")
cmd/gcs/main.go:392:		logrus.WithError(err).Fatal("failed to register memory threshold for gcs cgroup")
cmd/gcs/main.go:399:		logrus.WithError(err).Fatal("failed to retrieve the container cgroups oom eventfd")
cmd/gcs/main.go:407:		logrus.WithError(err).Fatal("failed to retrieve the virtual-pods cgroups oom eventfd")
cmd/gcs/main.go:415:			logrus.WithError(err).Fatal("failed to start time synchronization service")
cmd/ncproxy/hcn.go:309:		log.G(ctx).WithField("networkName", network.Name).Warn("network has multiple MAC pools, only returning the first")
cmd/ncproxy/hcn_networking_test.go:935:func TestAddEndpoint_NoError(t *testing.T) {
cmd/ncproxy/hcn_networking_test.go:1161:func TestDeleteEndpoint_NoError(t *testing.T) {
cmd/ncproxy/hcn_networking_test.go:1288:func TestDeleteNetwork_NoError(t *testing.T) {
cmd/ncproxy/hcn_networking_test.go:1387:func TestGetEndpoint_NoError(t *testing.T) {
cmd/ncproxy/hcn_networking_test.go:1493:func TestGetEndpoints_NoError(t *testing.T) {
cmd/ncproxy/hcn_networking_test.go:1556:func TestGetNetwork_NoError(t *testing.T) {
cmd/ncproxy/hcn_networking_test.go:1665:func TestGetNetworks_NoError(t *testing.T) {
cmd/ncproxy/ncproxy.go:108:			log.G(ctx).WithError(err).Warn("Failed to query ncproxy networking database")
cmd/ncproxy/ncproxy.go:125:			log.G(ctx).WithField("iov settings", settings.Policies.IovPolicySettings).Info("AddNIC iov settings")
cmd/ncproxy/ncproxy.go:173:		return nil, status.Error(codes.InvalidArgument, "received empty field in request")
cmd/ncproxy/ncproxy.go:199:		return nil, status.Error(codes.InvalidArgument, "received empty field in request")
cmd/ncproxy/ncproxy.go:201:	log.G(ctx).WithField("iov settings", settings.Policies.IovPolicySettings).Info("ModifyNIC iov settings")
cmd/ncproxy/ncproxy.go:279:			log.G(ctx).WithError(err).Warn("Failed to query ncproxy networking database")
cmd/ncproxy/ncproxy.go:459:			log.G(ctx).WithError(err).Warn("Failed to query ncproxy networking database")
cmd/ncproxy/ncproxy.go:480:			log.G(ctx).WithField("namespaceID", req.NamespaceID).Debug("Attaching endpoint to default host namespace")
cmd/ncproxy/ncproxy.go:511:			log.G(ctx).WithError(err).Warn("Failed to query ncproxy networking database")
cmd/ncproxy/ncproxy.go:546:			log.G(ctx).WithError(err).Warn("Failed to query ncproxy networking database")
cmd/ncproxy/ncproxy.go:612:		log.G(ctx).WithError(err).Warn("Failed to query ncproxy networking database")
cmd/ncproxy/ncproxy.go:691:		log.G(ctx).WithError(err).Warn("Failed to query ncproxy networking database")
cmd/ncproxy/ncproxy.go:812:		log.G(ctx).WithField("key", req.ContainerID).WithError(err).Warn("failed to delete key from compute agent store")
cmd/ncproxy/ncproxy.go:839:		return nil, status.Error(codes.InvalidArgument, "ContainerID is empty")
cmd/ncproxy/ncproxy.go:843:		return nil, status.Error(codes.FailedPrecondition, "No NodeNetworkService client registered")
cmd/ncproxy/ncproxy_networking_test.go:221:func TestModifyNIC_NCProxy_Returns_Error(t *testing.T) {
cmd/ncproxy/ncproxy_v0_service_test.go:673:func TestAddEndpoint_V0_NoError(t *testing.T) {
cmd/ncproxy/ncproxy_v0_service_test.go:852:func TestDeleteEndpoint_V0_NoError(t *testing.T) {
cmd/ncproxy/ncproxy_v0_service_test.go:963:func TestDeleteNetwork_V0_NoError(t *testing.T) {
cmd/ncproxy/ncproxy_v0_service_test.go:1046:func TestGetEndpoint_V0_NoError(t *testing.T) {
cmd/ncproxy/ncproxy_v0_service_test.go:1136:func TestGetEndpoints_V0_NoError(t *testing.T) {
cmd/ncproxy/ncproxy_v0_service_test.go:1181:func TestGetNetwork_V0_NoError(t *testing.T) {
cmd/ncproxy/ncproxy_v0_service_test.go:1264:func TestGetNetworks_V0_NoError(t *testing.T) {
cmd/ncproxy/run.go:52:			log.G(ctx).Info("falling back to v0 nodenetsvc api")
cmd/ncproxy/run.go:90:		logrus.WithField("stack", stacks).Info("ncproxy goroutine stack dump")
cmd/ncproxy/run.go:155:			logrus.Error(err)
cmd/ncproxy/run.go:158:		logrus.Error(err)
cmd/ncproxy/run.go:282:	}).Info("starting ncproxy")
cmd/ncproxy/run.go:306:		log.G(ctx).Info("Received interrupt. Closing")
cmd/ncproxy/run.go:312:		log.G(ctx).Info("Windows service stopped or shutdown")
cmd/ncproxy/server.go:53:		log.G(ctx).WithError(err).Error("failed to create ttrpc server")
cmd/ncproxy/server.go:80:		log.G(ctx).WithError(err).Errorf("failed to listen on %s", s.conf.TTRPCAddr)
cmd/ncproxy/server.go:86:		log.G(ctx).WithError(err).Errorf("failed to listen on %s", s.conf.GRPCAddr)
cmd/ncproxy/server.go:96:		log.G(ctx).WithError(err).Error("failed to gracefully shutdown ttrpc server")
cmd/ncproxy/server.go:103:		log.G(ctx).WithError(err).Error("failed to disconnect connections in compute agent cache")
cmd/ncproxy/server.go:106:		log.G(ctx).WithError(err).Error("failed to close ncproxy compute agent database")
cmd/ncproxy/server.go:109:		log.G(ctx).WithError(err).Error("failed to close ncproxy networking database")
cmd/ncproxy/server.go:114:	if err == nil || strings.Contains(err.Error(), "use of closed network connection") {
cmd/ncproxy/server.go:124:		}).Info("Serving ncproxy TTRPC service")
cmd/ncproxy/server.go:133:		}).Info("Serving ncproxy GRPC service")
cmd/ncproxy/server.go:170:		log.G(ctx).WithError(err).Debug("no entries in database")
cmd/ncproxy/server.go:173:		log.G(ctx).WithError(err).Error("failed to get compute agent information")
cmd/ncproxy/server.go:184:				log.G(ctx).WithField("agentAddress", agentAddress).WithError(err).Error("failed to create new compute agent client")
cmd/ncproxy/server.go:187:					log.G(ctx).WithField("key", containerID).WithError(dErr).Warn("failed to delete key from compute agent store")
cmd/ncproxy/server.go:191:			log.G(ctx).WithField("containerID", containerID).Info("reconnected to container's compute agent")
cmd/ncproxy/server.go:212:			log.G(ctx).WithError(err).Error("failed to close compute agent connection")
cmd/ncproxy/server_test.go:185:	mockedClient.EXPECT().ConfigureNetworking(gomock.Any(), gomock.Any()).Return(nil, status.Error(codes.Unimplemented, "mock the v1 api not implemented")).AnyTimes()
cmd/runhcs/container.go:527:	}).Info("creating container in UVM")
cmd/runhcs/container.go:688:		if !strings.Contains(err.Error(), "operation is not valid in the current state") {
cmd/runhcs/main.go:62:		logrus.Error(err)
cmd/runhcs/main.go:184:	logrus.Error(string(p))
cmd/runhcs/shim.go:148:				return cli.NewExitError("", 1)
cmd/runhcs/shim.go:229:		return cli.NewExitError("", code)
cmd/runhcs/vm.go:111:					logrus.Error(err)
cmd/runhcs/vm.go:136:					logrus.WithError(err).
cmd/runhcs/vm.go:137:						Error("failed creating container in VM")
cmd/runhcs/vm.go:155:	}).Debug("process request")
cmd/runhcs/vm.go:196:func (err *noVMError) Error() string {
cmd/shimdiag/exec.go:105:		return cli.NewExitError(errors.New(""), int(resp.ExitCode))
internal/bridgeutils/commonutils/utilities.go:43:	errorMessage = errForResponse.Error()
internal/bridgeutils/commonutils/utilities.go:62:			}).Error("opengcs::bridge::setErrorForResponseBase - failed to parse line number, using -1 instead")
internal/bridgeutils/gcserr/errors.go:97:func (e *baseHresultError) Error() string {
internal/bridgeutils/gcserr/errors.go:109:func (e *wrappingHresultError) Error() string {
internal/bridgeutils/gcserr/errors.go:110:	return fmt.Sprintf("HRESULT 0x%x", uint32(e.Hresult())) + ": " + e.Cause().Error()
internal/bridgeutils/gcserr/errors.go:127:		_, _ = io.WriteString(s, e.Error())
internal/bridgeutils/gcserr/errors.go:129:		fmt.Fprintf(s, "%q", e.Error())
internal/bridgeutils/gcserr/errors.go:144:func NewHresultError(hresult Hresult) error {
internal/builder/vm/lcow/boot.go:25:	log.G(ctx).Debug("resolveBootFilesPath: starting boot files path resolution")
internal/builder/vm/lcow/boot.go:48:	log.G(ctx).WithField(logfields.Path, bootFilesRootPath).Debug("resolveBootFilesPath completed successfully")
internal/builder/vm/lcow/boot.go:55:	log.G(ctx).Debug("parseBootOptions: starting boot options parsing")
internal/builder/vm/lcow/boot.go:63:	log.G(ctx).WithField(logfields.Path, bootFilesPath).Debug("using boot files path")
internal/builder/vm/lcow/boot.go:82:		).Debug("updated LCOW root filesystem to " + vmutils.VhdFile)
internal/builder/vm/lcow/boot.go:95:	}).Debug("determined boot mode")
internal/builder/vm/lcow/boot.go:108:			log.G(ctx).WithField(vmutils.UncompressedKernelFile, filepath.Join(bootFilesPath, vmutils.UncompressedKernelFile)).Debug("updated LCOW kernel file to " + vmutils.UncompressedKernelFile)
internal/builder/vm/lcow/boot.go:121:	log.G(ctx).WithField("kernelFile", kernelFileName).Debug("selected kernel file")
internal/builder/vm/lcow/boot.go:125:		log.G(ctx).WithField("preferredRootFSType", preferredRootfsType).Debug("applying preferred rootfs type override")
internal/builder/vm/lcow/boot.go:139:	log.G(ctx).WithField("rootFsFile", rootFsFile).Debug("selected rootfs file")
internal/builder/vm/lcow/boot.go:143:		log.G(ctx).Debug("configuring kernel direct boot")
internal/builder/vm/lcow/boot.go:150:			log.G(ctx).WithField("initrdPath", chipset.LinuxKernelDirect.InitRdPath).Debug("configured initrd for kernel direct boot")
internal/builder/vm/lcow/boot.go:154:		log.G(ctx).Debug("configuring UEFI boot")
internal/builder/vm/lcow/boot.go:169:	}).Debug("parseBootOptions completed successfully")
internal/builder/vm/lcow/confidential.go:41:	log.G(ctx).Debug("parseConfidentialOptions: starting confidential options parsing")
internal/builder/vm/lcow/confidential.go:65:	log.G(ctx).WithField("vmgsTemplatePath", vmgsTemplatePath).Debug("VMGS template path configured")
internal/builder/vm/lcow/confidential.go:81:	log.G(ctx).WithField("dmVerityRootfsPath", dmVerityRootfsTemplatePath).Debug("DM Verity rootfs path configured")
internal/builder/vm/lcow/confidential.go:89:	log.G(ctx).Debug("configuring UEFI secure boot for confidential VM")
internal/builder/vm/lcow/confidential.go:100:	log.G(ctx).Debug("creating security policy digest")
internal/builder/vm/lcow/confidential.go:125:	log.G(ctx).Debug("configuring HvSocket service table for confidential VM")
internal/builder/vm/lcow/confidential.go:144:	}).Debug("configured VSock ports for confidential VM")
internal/builder/vm/lcow/confidential.go:160:	log.G(ctx).Debug("parseConfidentialOptions completed successfully")
internal/builder/vm/lcow/confidential.go:172:	log.G(ctx).Debug("setGuestState: starting guest state configuration")
internal/builder/vm/lcow/confidential.go:180:	}).Debug("copying VMGS template file")
internal/builder/vm/lcow/confidential.go:192:	}).Debug("copying DM-Verity rootfs template file")
internal/builder/vm/lcow/confidential.go:203:	log.G(ctx).Debug("granting VM group access to confidential files")
internal/builder/vm/lcow/confidential.go:232:	}).Debug("configured SCSI attachment for dm-verity rootfs in confidential mode")
internal/builder/vm/lcow/confidential.go:242:	log.G(ctx).Debug("setGuestState completed successfully")
internal/builder/vm/lcow/devices.go:42:	}).Debug("parseDeviceOptions: starting device options parsing")
internal/builder/vm/lcow/devices.go:64:	}).Debug("parsed VPMem configuration")
internal/builder/vm/lcow/devices.go:100:			}).Debug("configured VPMem device for VHD rootfs boot")
internal/builder/vm/lcow/devices.go:112:	log.G(ctx).WithField("scsiControllerCount", scsiControllerCount).Debug("configuring SCSI controllers")
internal/builder/vm/lcow/devices.go:135:		}).Debug("configured SCSI attachment for VHD rootfs boot")
internal/builder/vm/lcow/devices.go:145:		log.G(ctx).Debug("parsing vPCI device assignments")
internal/builder/vm/lcow/devices.go:162:				log.G(ctx).Debug("NUMA affinity propagation enabled for vPCI devices")
internal/builder/vm/lcow/devices.go:191:				}).Debug("configured vPCI device")
internal/builder/vm/lcow/devices.go:196:	log.G(ctx).Debug("parseDeviceOptions completed successfully")
internal/builder/vm/lcow/devices.go:208:		}).Debug("getVPCIDevice: resolved valid vPCI device")
internal/builder/vm/lcow/kernel_args.go:32:	log.G(ctx).WithField("rootFsFile", rootFsFile).Debug("buildKernelArgs: starting kernel arguments construction")
internal/builder/vm/lcow/kernel_args.go:88:	log.G(ctx).WithField("kernelArgs", result).Debug("buildKernelArgs completed successfully")
internal/builder/vm/lcow/kernel_args.go:104:	}).Debug("buildRootfsArgs: starting rootfs args construction")
internal/builder/vm/lcow/kernel_args.go:164:	}).Debug("buildInitArgs: starting init args construction")
internal/builder/vm/lcow/kernel_args.go:179:			log.G(ctx).Warn("ignoring `WritableOverlayDirs` option since rootfs is already writable")
internal/builder/vm/lcow/kernel_args.go:194:	log.G(ctx).Debug("buildInitArgs completed successfully")
internal/builder/vm/lcow/specs.go:36:	log.G(ctx).Info("BuildSandboxConfig: starting sandbox spec generation")
internal/builder/vm/lcow/specs.go:54:	log.G(ctx).WithField("platform", platform).Debug("validating sandbox platform")
internal/builder/vm/lcow/specs.go:194:		log.G(ctx).Debug("disabling memory overcommit for confidential VM")
internal/builder/vm/lcow/specs.go:226:		log.G(ctx).WithField("kernelArgs", kernelArgs).Debug("kernel arguments configured")
internal/builder/vm/lcow/specs.go:233:	log.G(ctx).Debug("assembling final sandbox hcs spec")
internal/builder/vm/lcow/specs.go:272:	log.G(ctx).Info("sandbox spec generation completed successfully")
internal/builder/vm/lcow/specs.go:279:	log.G(ctx).WithField("annotationCount", len(annotations)).Debug("processAnnotations: starting annotations processing")
internal/builder/vm/lcow/specs.go:300:	log.G(ctx).Debug("processAnnotations completed successfully")
internal/builder/vm/lcow/specs.go:308:	log.G(ctx).WithField("platform", platform).Debug("parseSandboxOptions: starting sandbox options parsing")
internal/builder/vm/lcow/specs.go:341:		}).Debug("determined confidential VM mode")
internal/builder/vm/lcow/specs.go:348:	log.G(ctx).Debug("parseSandboxOptions completed successfully")
internal/builder/vm/lcow/specs.go:354:	log.G(ctx).Debug("parseStorageQOSOptions: starting storage QOS options parsing")
internal/builder/vm/lcow/specs.go:362:	}).Debug("parseStorageQOSOptions completed successfully")
internal/builder/vm/lcow/specs.go:376:	log.G(ctx).Debug("setAdditionalOptions: starting additional options parsing")
internal/builder/vm/lcow/specs.go:406:	log.G(ctx).Debug("setAdditionalOptions completed successfully")
internal/builder/vm/lcow/specs_test.go:60:				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
internal/builder/vm/lcow/specs_test.go:61:					t.Errorf("expected error containing %q, got %q", tt.errContains, err.Error())
internal/builder/vm/lcow/specs_test.go:96:	processorTopology, err := processorinfo.HostProcessorInfo(ctx)
internal/builder/vm/lcow/specs_test.go:343:					t.Error("expected kernel direct boot (LinuxKernelDirect to be set)")
internal/builder/vm/lcow/specs_test.go:369:					t.Error("expected InitRdPath to be set for initrd boot")
internal/builder/vm/lcow/specs_test.go:414:					t.Error("expected Devices to be configured")
internal/builder/vm/lcow/specs_test.go:521:						t.Error("expected at least one function in VirtualPciDevice")
internal/builder/vm/lcow/specs_test.go:574:						t.Error("expected at least one function")
internal/builder/vm/lcow/specs_test.go:613:					t.Error("expected GuestState file path to be set in confidential mode")
internal/builder/vm/lcow/specs_test.go:617:					t.Error("expected SCSI controllers to be configured in confidential mode")
internal/builder/vm/lcow/specs_test.go:648:					t.Error("expected GuestState file path to be set in SNP mode")
internal/builder/vm/lcow/specs_test.go:652:					t.Error("expected SCSI controllers to be configured in SNP mode")
internal/builder/vm/lcow/specs_test.go:863:					t.Error("expected no writable file shares")
internal/builder/vm/lcow/specs_test.go:866:					t.Error("expected kernel direct boot (LinuxKernelDirect to be set)")
internal/builder/vm/lcow/specs_test.go:950:					t.Error("expected VirtualPMem to be configured")
internal/builder/vm/lcow/specs_test.go:973:					t.Error("expected VirtualPMem to be configured")
internal/builder/vm/lcow/specs_test.go:1031:					t.Error("expected boot options (Chipset) to be set")
internal/builder/vm/lcow/specs_test.go:1148:					t.Error("expected VPMem disabled in SNP mode")
internal/builder/vm/lcow/specs_test.go:1152:					t.Error("expected overcommit disabled in SNP mode")
internal/builder/vm/lcow/specs_test.go:1156:					t.Error("expected GuestState file path to be set in SNP mode")
internal/builder/vm/lcow/specs_test.go:1160:					t.Error("expected SCSI controllers to be set in SNP mode")
internal/builder/vm/lcow/specs_test.go:1178:					t.Error("expected VPMem NOT disabled when no security hardware")
internal/builder/vm/lcow/specs_test.go:1195:					t.Error("expected scratch encryption disabled by default when security hardware is bypassed")
internal/builder/vm/lcow/specs_test.go:1211:					t.Error("expected scratch encryption disabled when explicitly set")
internal/builder/vm/lcow/specs_test.go:1220:					t.Error("expected scratch encryption disabled without security policy")
internal/builder/vm/lcow/specs_test.go:1492:					t.Error("expected InitRdPath to be empty when VHD is default")
internal/builder/vm/lcow/specs_test.go:1506:					t.Error("expected InitRdPath to be set when only initrd is present")
internal/builder/vm/lcow/specs_test.go:1569:					t.Error("expected UEFI boot mode")
internal/builder/vm/lcow/specs_test.go:1572:					t.Error("expected LinuxKernelDirect to be nil for UEFI boot")
internal/builder/vm/lcow/specs_test.go:1592:					t.Error("expected dm-verity configuration in kernel args")
internal/builder/vm/lcow/specs_test.go:1610:					t.Error("expected SCSI controllers to be configured")
internal/builder/vm/lcow/specs_test.go:1632:					t.Error("expected VPMem device to be configured for rootfs")
internal/builder/vm/lcow/specs_test.go:1639:					t.Error("expected VPMem device to be read-only")
internal/builder/vm/lcow/specs_test.go:1685:					t.Error("expected -w flag in kernel args for writable overlay dirs")
internal/builder/vm/lcow/specs_test.go:1703:					t.Error("expected -disable-time-sync flag in kernel args")
internal/builder/vm/lcow/specs_test.go:1722:					t.Error("expected -core-dump-location in kernel args")
internal/builder/vm/lcow/specs_test.go:1740:					t.Error("expected PCI to be enabled, but found pci=off in kernel args")
internal/builder/vm/lcow/specs_test.go:1753:					t.Error("expected pci=off in kernel args when VPCIEnabled is false")
internal/builder/vm/lcow/specs_test.go:1767:					t.Error("expected -loglevel debug in kernel args")
internal/builder/vm/lcow/specs_test.go:1781:					t.Error("expected -scrub-logs in kernel args")
internal/builder/vm/lcow/specs_test.go:1848:					t.Error("expected vPCI devices to be disabled in confidential VMs")
internal/builder/vm/lcow/specs_test.go:1912:					t.Error("expected HvSocket service GUID to be in service table")
internal/builder/vm/lcow/specs_test.go:1979:					t.Error("expected HclEnabled to be true")
internal/builder/vm/lcow/specs_test.go:1983:					t.Error("expected GuestState file path to be set in confidential mode")
internal/builder/vm/lcow/specs_test.go:2140:	if !strings.Contains(err.Error(), "vNUMA topology is not supported") {
internal/builder/vm/lcow/specs_test.go:2141:		t.Errorf("expected error containing %q, got %q", "vNUMA topology is not supported", err.Error())
internal/builder/vm/lcow/topology.go:24:	log.G(ctx).Debug("parseCPUOptions: starting CPU options parsing")
internal/builder/vm/lcow/topology.go:31:	processorTopology, err := processorinfo.HostProcessorInfo(ctx)
internal/builder/vm/lcow/topology.go:62:		log.G(ctx).WithField("resourcePartitionID", resourcePartitionID).Debug("setting resource partition ID")
internal/builder/vm/lcow/topology.go:79:	}).Debug("parseCPUOptions completed successfully")
internal/builder/vm/lcow/topology.go:86:	log.G(ctx).Debug("parseMemoryOptions: starting memory options parsing")
internal/builder/vm/lcow/topology.go:122:	}).Debug("parseMemoryOptions completed successfully")
internal/builder/vm/lcow/topology.go:130:	log.G(ctx).Debug("parseNUMAOptions: starting NUMA options parsing")
internal/builder/vm/lcow/topology.go:149:		log.G(ctx).WithField("virtualNodeCount", hcsNuma.VirtualNodeCount).Debug("vNUMA topology configured")
internal/builder/vm/lcow/topology.go:161:	}).Debug("parseNUMAOptions completed successfully")
internal/cmd/cmd.go:92:func (err *ExitError) Error() string {
internal/cmd/cmd.go:228:				c.Log.WithError(err).Warn("failed to close Cmd stdin")
internal/cmd/cmd.go:237:				c.Log.WithError(cErr).Warn("failed to close Cmd stdout")
internal/cmd/cmd.go:247:				c.Log.WithError(cErr).Warn("failed to close Cmd stderr")
internal/cmd/cmd.go:278:		c.Log.WithError(waitErr).Warn("process wait failed")
internal/cmd/cmd.go:306:					c.Log.WithField("timeout", c.CopyAfterExitTimeout).Warn(err.Error())
internal/cmd/io.go:77:			log = log.WithError(err)
internal/cmd/io_binary.go:70:			log.G(ctx).WithError(err).Errorf("error closing wait pipe: %s", waitPipePath)
internal/cmd/io_binary.go:115:	}).Debug("binary io process started")
internal/cmd/io_binary.go:186:				log.G(ctx).WithError(err).Errorf("error while closing stdout npipe")
internal/cmd/io_binary.go:192:				log.G(ctx).WithError(err).Errorf("error while closing stderr npipe")
internal/cmd/io_binary.go:205:				log.G(ctx).WithError(err).Errorf("error while waiting for binary cmd to finish")
internal/cmd/io_binary.go:211:				log.G(ctx).WithError(err).Errorf("error while killing binaryIO process")
internal/cmd/io_binary.go:293:		log.G(context.TODO()).WithError(err).Debug("error closing pipe listener")
internal/cmd/io_npipe.go:31:	}).Debug("NewNpipeIO")
internal/cmd/io_npipe.go:114:				}).Error("Named pipe disconnected, retrying dial")
internal/cmd/io_npipe.go:120:					log.G(nprw.ctx).WithField("address", nprw.pipePath).Info("Succeeded in reconnecting to named pipe")
internal/cmd/io_npipe.go:197:			log.G(ctx).Debug("npipeio::sinCloser")
internal/cmd/io_npipe.go:203:			log.G(ctx).Debug("npipeio::outErrCloser - stdout")
internal/cmd/io_npipe.go:207:			log.G(ctx).Debug("npipeio::outErrCloser - stderr")
internal/cmd/io_npipe.go:216:			log.G(ctx).Debug("npipeio::sinCloser")
internal/cmd/io_npipe.go:263:			logrus.WithError(err).Error("failed to accept pipe")
internal/cni/registry.go:94:			if regstate.IsNotFoundError(err) {
internal/cni/registry.go:103:			if regstate.IsNotFoundError(err) {
internal/cni/registry_test.go:29:		if !regstate.IsNotFoundError(err) {
internal/computeagent/computeagent.pb.go:39:	ms.StoreMessageInfo(mi)
internal/computeagent/computeagent.pb.go:52:		if ms.LoadMessageInfo() == nil {
internal/computeagent/computeagent.pb.go:53:			ms.StoreMessageInfo(mi)
internal/computeagent/computeagent.pb.go:104:	ms.StoreMessageInfo(mi)
internal/computeagent/computeagent.pb.go:117:		if ms.LoadMessageInfo() == nil {
internal/computeagent/computeagent.pb.go:118:			ms.StoreMessageInfo(mi)
internal/computeagent/computeagent.pb.go:150:	ms.StoreMessageInfo(mi)
internal/computeagent/computeagent.pb.go:163:		if ms.LoadMessageInfo() == nil {
internal/computeagent/computeagent.pb.go:164:			ms.StoreMessageInfo(mi)
internal/computeagent/computeagent.pb.go:207:	ms.StoreMessageInfo(mi)
internal/computeagent/computeagent.pb.go:220:		if ms.LoadMessageInfo() == nil {
internal/computeagent/computeagent.pb.go:221:			ms.StoreMessageInfo(mi)
internal/computeagent/computeagent.pb.go:246:	ms.StoreMessageInfo(mi)
internal/computeagent/computeagent.pb.go:259:		if ms.LoadMessageInfo() == nil {
internal/computeagent/computeagent.pb.go:260:			ms.StoreMessageInfo(mi)
internal/computeagent/computeagent.pb.go:303:	ms.StoreMessageInfo(mi)
internal/computeagent/computeagent.pb.go:316:		if ms.LoadMessageInfo() == nil {
internal/computeagent/computeagent.pb.go:317:			ms.StoreMessageInfo(mi)
internal/computeagent/computeagent.pb.go:342:	ms.StoreMessageInfo(mi)
internal/computeagent/computeagent.pb.go:355:		if ms.LoadMessageInfo() == nil {
internal/computeagent/computeagent.pb.go:356:			ms.StoreMessageInfo(mi)
internal/computeagent/computeagent.pb.go:399:	ms.StoreMessageInfo(mi)
internal/computeagent/computeagent.pb.go:412:		if ms.LoadMessageInfo() == nil {
internal/computeagent/computeagent.pb.go:413:			ms.StoreMessageInfo(mi)
internal/computeagent/computeagent.pb.go:438:	ms.StoreMessageInfo(mi)
internal/computeagent/computeagent.pb.go:451:		if ms.LoadMessageInfo() == nil {
internal/computeagent/computeagent.pb.go:452:			ms.StoreMessageInfo(mi)
internal/computeagent/computeagent.pb.go:495:	ms.StoreMessageInfo(mi)
internal/computeagent/computeagent.pb.go:508:		if ms.LoadMessageInfo() == nil {
internal/computeagent/computeagent.pb.go:509:			ms.StoreMessageInfo(mi)
internal/computeagent/computeagent.pb.go:534:	ms.StoreMessageInfo(mi)
internal/computeagent/computeagent.pb.go:547:		if ms.LoadMessageInfo() == nil {
internal/computeagent/computeagent.pb.go:548:			ms.StoreMessageInfo(mi)
internal/computecore/computecore.go:34://sys hcsGetOperationResultAndProcessInfo(operation HcsOperation, processInformation *HcsProcessInformation, resultDocument **uint16) (hr error) = computecore.HcsGetOperationResultAndProcessInfo?
internal/computecore/computecore.go:38://sys hcsWaitForOperationResultAndProcessInfo(operation HcsOperation, timeoutMs uint32, processInformation *HcsProcessInformation, resultDocument **uint16) (hr error) = computecore.HcsWaitForOperationResultAndProcessInfo?
internal/computecore/computecore.go:75://sys hcsGetProcessInfo(process HcsProcess, operation HcsOperation) (hr error) = computecore.HcsGetProcessInfo?
internal/computecore/computecore.go:259:func HcsGetOperationResultAndProcessInfo(ctx gcontext.Context, operation HcsOperation) (processInformation HcsProcessInformation, resultDocument string, hr error) {
internal/computecore/computecore.go:271:		err := hcsGetOperationResultAndProcessInfo(operation, &processInformation, &resultDocumentp)
internal/computecore/computecore.go:326:func HcsWaitForOperationResultAndProcessInfo(ctx gcontext.Context, operation HcsOperation, timeoutMs uint32) (processInformation HcsProcessInformation, resultDocument string, hr error) {
internal/computecore/computecore.go:338:		err := hcsWaitForOperationResultAndProcessInfo(operation, timeoutMs, &processInformation, &resultDocumentp)
internal/computecore/computecore.go:730:func HcsGetProcessInfo(ctx gcontext.Context, process HcsProcess, operation HcsOperation) (hr error) {
internal/computecore/computecore.go:736:		return hcsGetProcessInfo(process, operation)
internal/computecore/zsyscall_windows.go:500:func hcsGetOperationResultAndProcessInfo(operation HcsOperation, processInformation *HcsProcessInformation, resultDocument **uint16) (hr error) {
internal/computecore/zsyscall_windows.go:527:func hcsGetProcessInfo(process HcsProcess, operation HcsOperation) (hr error) {
internal/computecore/zsyscall_windows.go:1238:func hcsWaitForOperationResultAndProcessInfo(operation HcsOperation, timeoutMs uint32, processInformation *HcsProcessInformation, resultDocument **uint16) (hr error) {
internal/controller/device/scsi/controller_test.go:143:		t.Error("expected different reservation IDs")
internal/controller/device/scsi/controller_test.go:227:		t.Error("expected non-empty guestPath")
internal/controller/device/scsi/controller_test.go:239:func TestMapToGuest_AttachError(t *testing.T) {
internal/controller/device/scsi/controller_test.go:252:func TestMapToGuest_MountError(t *testing.T) {
internal/controller/device/scsi/controller_test.go:307:func TestUnmapFromGuest_UnmountError(t *testing.T) {
internal/controller/device/scsi/controller_test.go:325:func TestUnmapFromGuest_DetachError(t *testing.T) {
internal/controller/device/scsi/disk/disk_test.go:127:		t.Error("expected equal configs to be equal")
internal/controller/device/scsi/disk/disk_test.go:133:		t.Error("expected different HostPath to be not equal")
internal/controller/device/scsi/disk/disk_test.go:139:		t.Error("expected different ReadOnly to be not equal")
internal/controller/device/scsi/disk/disk_test.go:145:		t.Error("expected different Type to be not equal")
internal/controller/device/scsi/disk/disk_test.go:151:		t.Error("expected different EVDType to be not equal")
internal/controller/device/scsi/disk/disk_test.go:166:func TestAttachToVM_FromReserved_Error(t *testing.T) {
internal/controller/device/scsi/disk/disk_test.go:227:func TestDetachFromVM_LinuxEjectError(t *testing.T) {
internal/controller/device/scsi/disk/disk_test.go:243:func TestDetachFromVM_RemoveError(t *testing.T) {
internal/controller/device/scsi/disk/disk_test.go:334:		t.Error("expected same mount object on duplicate reservation")
internal/controller/device/scsi/disk/disk_test.go:373:		t.Error("expected non-empty guestPath")
internal/controller/device/scsi/disk/disk_test.go:393:func TestMountPartitionToGuest_MountError(t *testing.T) {
internal/controller/device/scsi/disk/disk_test.go:442:func TestUnmountPartitionFromGuest_UnmountError(t *testing.T) {
internal/controller/device/scsi/disk/disk_test.go:542:		t.Error("expected non-empty guestPath")
internal/controller/device/scsi/disk/disk_test.go:546:func TestMountPartitionToGuest_WCOW_MountError(t *testing.T) {
internal/controller/device/scsi/disk/disk_test.go:571:		t.Error("expected non-empty guestPath")
internal/controller/device/scsi/disk/disk_test.go:592:func TestUnmountPartitionFromGuest_WCOW_UnmountError(t *testing.T) {
internal/controller/device/scsi/mount/mount_test.go:117:		t.Error("expected equal configs to be equal")
internal/controller/device/scsi/mount/mount_test.go:187:		t.Error("expected non-empty guestPath")
internal/controller/device/scsi/mount/mount_test.go:201:		t.Error("expected non-empty guestPath")
internal/controller/device/scsi/mount/mount_test.go:208:func TestMountToGuest_LCOW_Error(t *testing.T) {
internal/controller/device/scsi/mount/mount_test.go:223:func TestMountToGuest_WCOW_Error(t *testing.T) {
internal/controller/device/scsi/mount/mount_test.go:247:		t.Error("expected AddWCOWMappedVirtualDiskForContainerScratch to be called")
internal/controller/device/scsi/mount/mount_test.go:258:		t.Error("expected non-empty guestPath on idempotent mount")
internal/controller/device/scsi/mount/mount_test.go:306:func TestUnmountFromGuest_LCOW_Error(t *testing.T) {
internal/controller/device/scsi/mount/mount_test.go:322:func TestUnmountFromGuest_WCOW_Error(t *testing.T) {
internal/controller/device/vpmem/controller_test.go:118:		t.Error("expected different reservation IDs")
internal/controller/device/vpmem/controller_test.go:189:		t.Error("expected non-empty guestPath")
internal/controller/device/vpmem/controller_test.go:201:func TestMount_AttachError(t *testing.T) {
internal/controller/device/vpmem/controller_test.go:214:func TestMount_MountError(t *testing.T) {
internal/controller/device/vpmem/controller_test.go:268:func TestUnmount_UnmountError(t *testing.T) {
internal/controller/device/vpmem/controller_test.go:286:func TestUnmount_DetachError(t *testing.T) {
internal/controller/device/vpmem/device/device_test.go:97:		t.Error("expected equal configs to be equal")
internal/controller/device/vpmem/device/device_test.go:103:		t.Error("expected different HostPath to be not equal")
internal/controller/device/vpmem/device/device_test.go:109:		t.Error("expected different ReadOnly to be not equal")
internal/controller/device/vpmem/device/device_test.go:115:		t.Error("expected different ImageFormat to be not equal")
internal/controller/device/vpmem/device/device_test.go:130:func TestAttachToVM_FromReserved_Error(t *testing.T) {
internal/controller/device/vpmem/device/device_test.go:179:func TestDetachFromVM_RemoveError(t *testing.T) {
internal/controller/device/vpmem/device/device_test.go:268:		t.Error("expected same mount object on duplicate reservation")
internal/controller/device/vpmem/device/device_test.go:293:		t.Error("expected non-empty guestPath")
internal/controller/device/vpmem/device/device_test.go:313:func TestMountToGuest_MountError(t *testing.T) {
internal/controller/device/vpmem/device/device_test.go:360:func TestUnmountFromGuest_UnmountError(t *testing.T) {
internal/controller/device/vpmem/mount/mount_test.go:60:		t.Error("expected equal configs to be equal")
internal/controller/device/vpmem/mount/mount_test.go:99:		t.Error("expected non-empty guestPath")
internal/controller/device/vpmem/mount/mount_test.go:106:func TestMountToGuest_Error(t *testing.T) {
internal/controller/device/vpmem/mount/mount_test.go:128:		t.Error("expected non-empty guestPath on idempotent mount")
internal/controller/device/vpmem/mount/mount_test.go:155:func TestUnmountFromGuest_Error(t *testing.T) {
internal/controller/vm/vm.go:220:		log.G(ctx).WithField("currentState", c.vmState).Debug("waitForVMExit: state transition to Terminated was a no-op")
internal/controller/vm/vm.go:347:		memCounters, err := process.GetProcessMemoryInfo(c.vmmemProcess)
internal/controller/vm/vm.go:440:		Err:         c.uvm.ExitError(),
internal/controller/vm/vm_wcow.go:54:			logrus.WithError(err).Error("failed to listen for windows logging connections")
internal/controller/vm/vm_wcow.go:71:				logrus.WithError(err).Error("failed to connect to log socket")
internal/controller/vm/vm_wcow.go:79:				logrus.Info("uvm output handler starting")
internal/controller/vm/vm_wcow.go:85:				logrus.Info("uvm output handler finished")
internal/cow/cow.go:96:	WaitError() error
internal/credentials/credentials.go:51:	log.G(ctx).WithField("containerID", id).Debug("creating container credential guard instance")
internal/credentials/credentials.go:118:	log.G(ctx).WithField("containerID", id).Debug("removing container credential guard")
internal/devices/assigned_devices.go:37:				log.G(ctx).WithError(releaseErr).Error("failed to release container resource")
internal/devices/assigned_devices.go:49:		log.G(ctx).WithField("vmbus id", vmBusInstanceID).Info("vmbus instance ID")
internal/devices/drivers.go:37:				log.G(ctx).WithError(releaseErr).Error("failed to release container resource")
internal/devices/pnp.go:64:		}).Warn("expected version of driver may not have been installed")
internal/devices/pnp.go:67:	log.G(ctx).WithField("added drivers", driverDir).Debug("installed drivers")
internal/extendedtask/extendedtask.pb.go:35:	ms.StoreMessageInfo(mi)
internal/extendedtask/extendedtask.pb.go:48:		if ms.LoadMessageInfo() == nil {
internal/extendedtask/extendedtask.pb.go:49:			ms.StoreMessageInfo(mi)
internal/extendedtask/extendedtask.pb.go:79:	ms.StoreMessageInfo(mi)
internal/extendedtask/extendedtask.pb.go:92:		if ms.LoadMessageInfo() == nil {
internal/extendedtask/extendedtask.pb.go:93:			ms.StoreMessageInfo(mi)
internal/extendedtask/extendedtask.proto:6:    rpc ComputeProcessorInfo(ComputeProcessorInfoRequest) returns (ComputeProcessorInfoResponse);
internal/extendedtask/extendedtask_ttrpc.pb.go:11:	ComputeProcessorInfo(context.Context, *ComputeProcessorInfoRequest) (*ComputeProcessorInfoResponse, error)
internal/extendedtask/extendedtask_ttrpc.pb.go:22:				return svc.ComputeProcessorInfo(ctx, &req)
internal/extendedtask/extendedtask_ttrpc.pb.go:38:func (c *extendedtaskClient) ComputeProcessorInfo(ctx context.Context, req *ComputeProcessorInfoRequest) (*ComputeProcessorInfoResponse, error) {
internal/gcs-sidecar/bridge.go:143:		}).Warn("overwriting bridge handler")
internal/gcs-sidecar/bridge.go:189:		logrus.WithError(err).Errorf("error reading message header")
internal/gcs-sidecar/bridge.go:212:func isLocalDisconnectError(err error) bool {
internal/gcs-sidecar/bridge.go:321:					if errors.Is(err, io.EOF) || isLocalDisconnectError(err) {
internal/gcs-sidecar/bridge.go:328:					logrus.Error(recverr)
internal/gcs-sidecar/bridge.go:349:					log.G(ctx).WithError(err).Error("failed to send request to shimRequestChan")
internal/gcs-sidecar/bridge.go:374:					log.G(req.ctx).WithError(err).Errorf("failed to serve request: %v", req.header.Type.String())
internal/gcs-sidecar/bridge.go:379:						ErrorMessage: err.Error(),
internal/gcs-sidecar/bridge.go:386:						log.G(req.ctx).WithError(err).Errorf("failed to send response to shim")
internal/gcs-sidecar/bridge.go:438:					if errors.Is(err, io.EOF) || isLocalDisconnectError(err) {
internal/gcs-sidecar/bridge.go:445:					logrus.Error(recverr)
internal/gcs-sidecar/bridge.go:455:						log.G(ctx).WithError(err).Error("failed to unmarshal the request")
internal/gcs-sidecar/bridge.go:473:					log.G(ctx).WithError(err).Error("failed to send request to b.sendToShimCh")
internal/gcs-sidecar/handlers.go:114:					log.G(ctx).WithError(err).Warn("Registry changes validation failed - rejecting")
internal/gcs-sidecar/handlers.go:150:					log.G(ctx).WithError(removeErr).Errorf("Failed to remove container: %v", containerID)
internal/gcs-sidecar/handlers.go:732:							}).Warn("waiting for block CIM device to show up")
internal/gcs-sidecar/handlers.go:744:					cimRootDigestBytes, err := cimfs.GetVerificationInfo(physicalDevPath)
internal/gcs-sidecar/host.go:71:		return gcserr.NewHresultError(gcserr.HrVmcomputeSystemAlreadyExists)
internal/gcs-sidecar/host.go:85:		return gcserr.NewHresultError(gcserr.HrVmcomputeSystemNotFound)
internal/gcs-sidecar/host.go:99:		return nil, gcserr.NewHresultError(gcserr.HrVmcomputeSystemNotFound)
internal/gcs-sidecar/host.go:111:	}).Info("opengcs::Container::GetProcess")
internal/gcs-sidecar/host.go:118:		return nil, gcserr.NewHresultError(gcserr.HrErrNotFound)
internal/gcs-sidecar/uvm.go:35:			log.G(ctx).Info("Unmarshalling log forward service modify settings")
internal/gcs-sidecar/vsmb.go:49:				logrus.WithError(derr).Warn("Failed to disconnect Service Manager")
internal/gcs-sidecar/vsmb.go:62:				logrus.WithError(derr).Warn("Failed to close LanmanWorkstation service")
internal/gcs-sidecar/vsmb.go:146:				logrus.WithError(derr).Warn("Failed to disconnect Service Manager")
internal/gcs-sidecar/vsmb.go:158:				logrus.WithError(derr).Warn("Failed to close LanmanWorkstation service")
internal/gcs-sidecar/vsmb.go:173:	logrus.Info("Starting VSMB initialization...")
internal/gcs-sidecar/vsmb.go:175:	logrus.Debug("Configuring LanmanWorkstation service...")
internal/gcs-sidecar/vsmb.go:186:		logrus.Info("LanmanWorkstation service is running.")
internal/gcs-sidecar/vsmb.go:188:		logrus.Warn("LanmanWorkstation service is NOT running.")
internal/gcs-sidecar/vsmb.go:198:		logrus.WithError(nerr).Errorf("invalid device name %q", GlobalRdrDeviceName)
internal/gcs-sidecar/vsmb.go:212:		logrus.WithError(err).Error("Failed to open redirector")
internal/gcs-sidecar/vsmb.go:217:			logrus.WithError(derr).Warn("Failed to close LanmanRedirector handle")
internal/gcs-sidecar/vsmb.go:221:	logrus.Info("Successfully opened LanmanRedirector device.")
internal/gcs-sidecar/vsmb.go:226:		logrus.WithError(nerr).Errorf("invalid instance name %q", GlobalVsmbInstanceName)
internal/gcs-sidecar/vsmb.go:271:		logrus.Info("VMSMB RDR instance started.")
internal/gcs-sidecar/vsmb.go:273:		logrus.Warn("VMSMB RDR instance already started.")
internal/gcs-sidecar/vsmb.go:281:		logrus.WithError(nerr).Errorf("invalid device name %q", GlobalVsmbDeviceName)
internal/gcs-sidecar/vsmb.go:296:			logrus.WithError(derr).Warn("Failed to close VSMB device handle")
internal/gcs-sidecar/vsmb.go:302:		logrus.WithError(nerr).Errorf("invalid instance name %q", GlobalVsmbTransportName)
internal/gcs-sidecar/vsmb.go:331:		logrus.Info("VMBUS transport bound to VMSMB RDR instance.")
internal/gcs-sidecar/vsmb.go:342:		logrus.WithError(nerr).Errorf("invalid device name %q", device)
internal/gcs-sidecar/vsmb.go:356:		logrus.WithError(err).Errorf("Failed to open %s", device)
internal/gcs/bridge.go:100:			brdg.log.WithError(err).Warn("bridge error, already terminated")
internal/gcs/bridge.go:108:		brdg.log.WithError(err).Error("bridge forcibly terminating")
internal/gcs/bridge.go:110:		brdg.log.Debug("bridge terminating")
internal/gcs/bridge.go:168:func (err *rpcError) Error() string {
internal/gcs/bridge.go:171:		msg = windows.Errno(err.result).Error()
internal/gcs/bridge.go:230:		brdg.log.WithField("reason", ctx.Err()).Warn("ignoring response to bridge message")
internal/gcs/bridge.go:282:func isLocalDisconnectError(err error) bool {
internal/gcs/bridge.go:291:			if err == io.EOF || isLocalDisconnectError(err) { //nolint:errorlint
internal/gcs/bridge.go:319:						"result-message": windows.Errno(rec.Result).Error(),
internal/gcs/bridge.go:326:					}).Error("bridge RPC error record")
internal/gcs/bridge.go:404:			brdg.log.WithError(err).Warning("could not scrub bridge payload")
internal/gcs/bridge.go:446:			brdg.log.WithError(err).Error("bridge write failed but call is already complete")
internal/gcs/bridge_test.go:45:		t.Error(err)
internal/gcs/bridge_test.go:50:		t.Error(err)
internal/gcs/bridge_test.go:62:				t.Error(err)
internal/gcs/bridge_test.go:112:	if err == nil || !strings.Contains(err.Error(), "bridge closed") {
internal/gcs/bridge_test.go:140:	if err == nil || !strings.Contains(err.Error(), "bridge closed") {
internal/gcs/bridge_test.go:190:		t.Error("notify failed: ", err)
internal/gcs/bridge_test.go:193:		t.Error("did not receive notification")
internal/gcs/bridge_test.go:203:	if err == nil || !strings.Contains(err.Error(), errMsg) {
internal/gcs/bridge_test.go:204:		t.Error("unexpected result: ", err)
internal/gcs/container.go:202:			log.G(ctx).WithError(err).Warn("ignoring missing container")
internal/gcs/container.go:240:func (c *Container) WaitError() error {
internal/gcs/container.go:248:	return c.WaitError()
internal/gcs/container.go:263:	log.G(ctx).Debug("container exited")
internal/gcs/guestconnection.go:290:	logrus.WithField(logfields.ContainerID, cid).Info("container terminated in guest")
internal/gcs/guestconnection.go:323:					log.G(ctx).WithError(err).Warn("failed to encode OpenCensus Tracestate")
internal/gcs/guestconnection_test.go:45:		t.Error(err)
internal/gcs/guestconnection_test.go:109:						t.Error(err)
internal/gcs/guestconnection_test.go:271:	if err == nil || (!strings.Contains(err.Error(), "bridge closed") && !strings.Contains(err.Error(), "bridge write")) {
internal/gcs/iochannel_test.go:30:	if err := <-ch; err == nil || !strings.Contains(err.Error(), "use of closed network connection") {
internal/gcs/iochannel_test.go:31:		t.Error("unexpected: ", err)
internal/gcs/process.go:110:	log.G(ctx).WithField("pid", p.id).Debug("created process pid")
internal/gcs/process.go:135:		log.G(ctx).WithError(err).Warn("close stdin failed")
internal/gcs/process.go:138:		log.G(ctx).WithError(err).Warn("close stdout failed")
internal/gcs/process.go:141:		log.G(ctx).WithError(err).Warn("close stderr failed")
internal/gcs/process.go:261:			}).Warn("ignoring missing process")
internal/gcs/process.go:290:		log.G(ctx).WithError(err).Error("failed wait")
internal/gcs/process.go:292:	log.G(ctx).WithField("exitCode", ec).Debug("process exited")
internal/guest/bridge/bridge.go:86:		}).Warn("opengcs::bridge - overwriting bridge handler")
internal/guest/bridge/bridge.go:325:						entry.WithError(err).Warning("could not scrub bridge payload")
internal/guest/bridge/bridge_unit_test.go:25:		t.Error("Failed to create bridge mux")
internal/guest/bridge/bridge_unit_test.go:32:		t.Error("Bridge mux map is not initialized")
internal/guest/bridge/bridge_unit_test.go:50:			t.Error("The code did not panic on nil handler")
internal/guest/bridge/bridge_unit_test.go:61:			t.Error("The code did not panic on nil map")
internal/guest/bridge/bridge_unit_test.go:78:		t.Error("The handler type map not successfully added.")
internal/guest/bridge/bridge_unit_test.go:83:		t.Error("The handler was not successfully added.")
internal/guest/bridge/bridge_unit_test.go:90:		t.Error("The handler added was not the same handler.")
internal/guest/bridge/bridge_unit_test.go:97:			t.Error("The code did not panic on nil handler")
internal/guest/bridge/bridge_unit_test.go:108:			t.Error("The code did not panic on nil handler")
internal/guest/bridge/bridge_unit_test.go:133:		t.Error("The handler type map not successfully added.")
internal/guest/bridge/bridge_unit_test.go:138:		t.Error("The handler was not successfully added.")
internal/guest/bridge/bridge_unit_test.go:145:		t.Error("The handler added was not the same handler.")
internal/guest/bridge/bridge_unit_test.go:152:			t.Error("The code did not panic on nil request to handler")
internal/guest/bridge/bridge_unit_test.go:244:		t.Error("Handler did not call the appropriate handler for a match request")
internal/guest/bridge/bridge_unit_test.go:276:		t.Error("Handler did not call the appropriate handler for a match request")
internal/guest/bridge/bridge_unit_test.go:337:		t.Error("Handler did not call the appropriate handler for a match request")
internal/guest/bridge/bridge_unit_test.go:368:		t.Error("Handler did not call the appropriate handler for a match request")
internal/guest/bridge/bridge_unit_test.go:471:			t.Error(err)
internal/guest/bridge/bridge_unit_test.go:485:		t.Error("Failed to send message to server")
internal/guest/bridge/bridge_unit_test.go:490:		t.Error("Failed to read message response from server")
internal/guest/bridge/bridge_unit_test.go:495:		t.Error("Failed to unmarshal response body from server")
internal/guest/bridge/bridge_unit_test.go:501:		t.Error("Response header was not resize console response.")
internal/guest/bridge/bridge_unit_test.go:504:		t.Error("Response header had wrong sequence id")
internal/guest/bridge/bridge_unit_test.go:557:			t.Error(err)
internal/guest/bridge/bridge_unit_test.go:565:		t.Error("Failed to send message to server")
internal/guest/bridge/bridge_unit_test.go:570:		t.Error("Failed to read message response from server")
internal/guest/bridge/bridge_unit_test.go:575:		t.Error("Failed to unmarshal response body from server")
internal/guest/bridge/bridge_unit_test.go:580:		t.Error("response header was not resize console response.")
internal/guest/bridge/bridge_unit_test.go:583:		t.Error("response header had wrong sequence id")
internal/guest/bridge/bridge_unit_test.go:586:		t.Error("response body did not have same activity id")
internal/guest/bridge/bridge_unit_test.go:589:		t.Error("response result was not 1 as expected")
internal/guest/bridge/bridge_unit_test.go:628:			t.Error(err)
internal/guest/bridge/bridge_unit_test.go:636:		t.Error("Failed to send first message to server")
internal/guest/bridge/bridge_unit_test.go:640:		t.Error("Failed to send second message to server")
internal/guest/bridge/bridge_unit_test.go:646:		t.Error("Failed to read first response from server")
internal/guest/bridge/bridge_unit_test.go:651:		t.Error("Failed to read first response from server")
internal/guest/bridge/bridge_unit_test.go:656:		t.Error("Incorrect response type for 2nd request")
internal/guest/bridge/bridge_unit_test.go:659:		t.Error("Incorrect response order for 2nd request")
internal/guest/bridge/bridge_unit_test.go:663:		t.Error("Incorrect response for 1st request")
internal/guest/bridge/bridge_unit_test.go:666:		t.Error("Incorrect response order for 1st request")
internal/guest/bridge/bridge_v2.go:60:		return nil, gcserr.NewHresultError(gcserr.HrVmcomputeUnsupportedProtocolVersion)
internal/guest/bridge/bridge_v2.go:202:	log.G(ctx).WithField("pid", pid).Debug("created process pid")
internal/guest/bridge/bridge_v2.go:387:		return nil, gcserr.NewHresultError(gcserr.HvVmcomputeTimeout)
internal/guest/kmsg/kmsg.go:111:		logrus.WithError(err).Error("failed to open /dev/kmsg")
internal/guest/kmsg/kmsg.go:126:				logrus.Warn("kmsg entry overwritten; skipping entry")
internal/guest/kmsg/kmsg.go:129:			logrus.WithError(err).Error("kmsg read failure")
internal/guest/kmsg/kmsg.go:138:			}).Error("failed to parse kmsg entry")
internal/guest/kmsg/kmsg.go:141:				logrus.WithFields(entry.logFormat()).Info("kmsg read")
internal/guest/network/netns.go:102:	entry.WithField("namespace", ns).Debug("New network namespace from PID")
internal/guest/network/netns.go:114:		entry.WithField("mtu", mtu).Debug("EncapOverhead non-zero, will set MTU")
internal/guest/network/netns.go:134:		entry.WithField("timeout", timeout.String()).Debug("Execing udhcpc with timeout...")
internal/guest/network/netns.go:160:				entry.WithError(err).Debugf("udhcpc failed [%s]", cos)
internal/guest/network/netns.go:201:		}).Debug("assigning IP address")
internal/guest/network/netns.go:284:			if strings.Contains(err.Error(), unreachableErrStr) && gw != nil {
internal/guest/network/netns_test.go:582:	if err == nil || !strings.Contains(err.Error(), unreachableErrStr) {
internal/guest/network/netns_test.go:648:	if err == nil || !strings.Contains(err.Error(), unreachableErrStr) {
internal/guest/network/network.go:142:	log.G(ctx).WithField("ifname", ifname).Debug("resolved ifname")
internal/guest/network/network_test.go:139:func (t *testDirEntry) Info() (os.FileInfo, error) {
internal/guest/runtime/hcsv2/container.go:84:	entity.Info("opengcs::Container::Start")
internal/guest/runtime/hcsv2/container.go:105:				}).Warn("failed to close log file")
internal/guest/runtime/hcsv2/container.go:136:	log.G(ctx).WithField(logfields.ContainerID, c.id).Info("opengcs::Container::ExecProcess")
internal/guest/runtime/hcsv2/container.go:186:	}).Info("opengcs::Container::GetProcess")
internal/guest/runtime/hcsv2/container.go:196:		return nil, gcserr.NewHresultError(gcserr.HrErrNotFound)
internal/guest/runtime/hcsv2/container.go:203:	log.G(ctx).WithField(logfields.ContainerID, c.id).Info("opengcs::Container::GetAllProcessPids")
internal/guest/runtime/hcsv2/container.go:220:	}).Info("opengcs::Container::Kill")
internal/guest/runtime/hcsv2/container.go:231:	entity.Info("opengcs::Container::Delete")
internal/guest/runtime/hcsv2/container.go:241:			entity.WithError(err).Error("failed to unmount sandbox mounts")
internal/guest/runtime/hcsv2/container.go:246:			entity.WithError(err).Error("failed to unmount tmpfs sandbox mounts")
internal/guest/runtime/hcsv2/container.go:251:			entity.WithError(err).Error("failed to unmount hugepages mounts")
internal/guest/runtime/hcsv2/container.go:280:	log.G(ctx).WithField(logfields.ContainerID, c.id).Info("opengcs::Container::Update")
internal/guest/runtime/hcsv2/nvidia_utils.go:75:		log.G(ctx).WithField("hook", log.Format(ctx, nvidiaHook)).Debug("adding nvidia device runtime hook")
internal/guest/runtime/hcsv2/process.go:99:			log.G(ctx).WithError(err).Error("failed to wait for runc process")
internal/guest/runtime/hcsv2/process.go:102:		log.G(ctx).WithField("exitCode", p.exitCode).Debug("process exited")
internal/guest/runtime/hcsv2/process.go:139:			return gcserr.NewHresultError(gcserr.HrErrNotFound)
internal/guest/runtime/hcsv2/process.go:198:			log.G(ctx).Debug("wait completed, releasing wait count")
internal/guest/runtime/hcsv2/process.go:205:				log.G(ctx).Debug("first wait completed, releasing first wait count")
internal/guest/runtime/hcsv2/process.go:218:			log.G(ctx).Debug("wait canceled before exit, releasing wait count")
internal/guest/runtime/hcsv2/process.go:247:		}).Debug("external process exited")
internal/guest/runtime/hcsv2/process.go:272:			return gcserr.NewHresultError(gcserr.HrErrNotFound)
internal/guest/runtime/hcsv2/uvm.go:164:		}).Info("Container removed from virtual pod")
internal/guest/runtime/hcsv2/uvm.go:182:		return nil, gcserr.NewHresultError(gcserr.HrVmcomputeSystemNotFound)
internal/guest/runtime/hcsv2/uvm.go:186:			gcserr.NewHresultError(gcserr.HrVmcomputeInvalidState))
internal/guest/runtime/hcsv2/uvm.go:196:		return gcserr.NewHresultError(gcserr.HrVmcomputeSystemAlreadyExists)
internal/guest/runtime/hcsv2/uvm.go:339:		}).Info("Virtual pod first container detected - treating as sandbox container")
internal/guest/runtime/hcsv2/uvm.go:369:		}).Info("Processing container for virtual pod")
internal/guest/runtime/hcsv2/uvm.go:469:					log.G(ctx).WithError(err).Debug("failed to add SEV device")
internal/guest/runtime/hcsv2/uvm.go:518:	user, groups, umask, err := h.securityOptions.PolicyEnforcer.GetUserInfo(settings.OCISpecification.Process, settings.OCISpecification.Root.Path)
internal/guest/runtime/hcsv2/uvm.go:572:		}).Debug("creating container log file parent directory in uVM")
internal/guest/runtime/hcsv2/uvm.go:663:			entry.WithError(err).Warning("could not scrub OCI spec written to config.json")
internal/guest/runtime/hcsv2/uvm.go:888:			user, groups, umask, err = h.securityOptions.PolicyEnforcer.GetUserInfo(params.OCIProcess, c.spec.Root.Path)
internal/guest/runtime/hcsv2/uvm.go:936:		return nil, gcserr.NewHresultError(gcserr.HrErrNotFound)
internal/guest/runtime/hcsv2/uvm.go:990:			log.G(ctx).WithField("propertyType", requestedProperty).Warn("unknown or empty property type")
internal/guest/runtime/hcsv2/uvm.go:1100:func newInvalidRequestTypeError(rt guestrequest.RequestType) error {
internal/guest/runtime/hcsv2/uvm.go:1117:		return newInvalidRequestTypeError(rt)
internal/guest/runtime/hcsv2/uvm.go:1203:		return newInvalidRequestTypeError(rt)
internal/guest/runtime/hcsv2/uvm.go:1230:		return newInvalidRequestTypeError(rt)
internal/guest/runtime/hcsv2/uvm.go:1266:		return newInvalidRequestTypeError(rt)
internal/guest/runtime/hcsv2/uvm.go:1275:		return newInvalidRequestTypeError(rt)
internal/guest/runtime/hcsv2/uvm.go:1356:		return newInvalidRequestTypeError(rt)
internal/guest/runtime/hcsv2/uvm.go:1377:		return newInvalidRequestTypeError(rt)
internal/guest/runtime/hcsv2/uvm.go:1431:	logrus.Info("Virtual pod support initialized")
internal/guest/runtime/hcsv2/uvm.go:1463:		}).Info("Creating virtual pod with specified resources")
internal/guest/runtime/hcsv2/uvm.go:1465:		logrus.WithField("virtualSandboxID", virtualSandboxID).Info("Creating pod cgroup with default resources as none were specified")
internal/guest/runtime/hcsv2/uvm.go:1491:	}).Info("Virtual pod created successfully")
internal/guest/runtime/hcsv2/uvm.go:1528:	}).Info("Container added to virtual pod")
internal/guest/runtime/hcsv2/uvm.go:1550:				logrus.WithError(err).WithField("virtualSandboxID", virtualSandboxID).
internal/guest/runtime/hcsv2/uvm.go:1551:					Warn("Failed to remove virtual pod network namespace (sandbox container removal)")
internal/guest/runtime/hcsv2/uvm.go:1566:	}).Info("Container removed from virtual pod")
internal/guest/runtime/hcsv2/uvm.go:1574:			logrus.WithError(err).WithField("virtualSandboxID", virtualSandboxID).
internal/guest/runtime/hcsv2/uvm.go:1575:				Warn("Failed to delete virtual pod cgroup")
internal/guest/runtime/hcsv2/uvm.go:1584:				logrus.WithError(err).WithField("virtualSandboxID", virtualSandboxID).
internal/guest/runtime/hcsv2/uvm.go:1585:					Warn("Failed to remove virtual pod network namespace")
internal/guest/runtime/hcsv2/uvm.go:1591:		logrus.WithField("virtualSandboxID", virtualSandboxID).Info("Virtual pod cleaned up")
internal/guest/runtime/runc/container.go:60:		runcErr := getRuncLogError(logPath)
internal/guest/runtime/runc/container.go:65:			logrus.Warn("runc start failed without writing error to log file")
internal/guest/runtime/runc/container.go:87:	}).Debug("runc::container::Kill")
internal/guest/runtime/runc/container.go:103:	logrus.WithField(logfields.ContainerID, c.id).Debug("runc::container::killAll")
internal/guest/runtime/runc/container.go:116:		runcErr := parseRuncError(string(out))
internal/guest/runtime/runc/container.go:125:	logrus.WithField(logfields.ContainerID, c.id).Debug("runc::container::Delete")
internal/guest/runtime/runc/container.go:129:		runcErr := parseRuncError(string(out))
internal/guest/runtime/runc/container.go:140:		runcErr := parseRuncError(string(out))
internal/guest/runtime/runc/container.go:153:		if runcErr := getRuncLogError(logPath); runcErr != nil {
internal/guest/runtime/runc/container.go:156:			logrus.Warn("runc resume failed without writing error to log file")
internal/guest/runtime/runc/container.go:168:		runcErr := parseRuncError(string(out))
internal/guest/runtime/runc/container.go:187:		runcErr := parseRuncError(string(out))
internal/guest/runtime/runc/container.go:251:	}).Debug("running container pids")
internal/guest/runtime/runc/container.go:330:			entity.WithField(logfields.ProcessID, process.Pid).Debug("waiting on container exec process")
internal/guest/runtime/runc/container.go:335:	entity.Debug("runc::container::init process wait completed")
internal/guest/runtime/runc/container.go:425:		if runcErr := getRuncLogError(logPath); runcErr != nil {
internal/guest/runtime/runc/container.go:428:			logrus.Warn("runc create/exec call failed without writing error to log file")
internal/guest/runtime/runc/container.go:478:		runcErr := parseRuncError(string(out))
internal/guest/runtime/runc/process.go:49:	l.WithField(logfields.ContainerID, p.pid).Debug("process wait completed")
internal/guest/runtime/runc/process.go:67:			l.WithError(err).Error("failed to terminate container after process wait")
internal/guest/runtime/runc/process.go:89:	l.WithField(logfields.ProcessID, p.pid).Debug("relay wait completed")
internal/guest/runtime/runc/runc.go:86:		runcErr := parseRuncError(string(out))
internal/guest/runtime/runc/runc.go:102:		runcErr := parseRuncError(string(out))
internal/guest/runtime/runc/utils.go:119:func (l *standardLogEntry) asError() (err error) {
internal/guest/runtime/runc/utils.go:120:	err = parseRuncError(l.Message)
internal/guest/runtime/runc/utils.go:122:		err = errors.Wrap(err, l.Err.Error())
internal/guest/runtime/runc/utils.go:127:func parseRuncError(s string) (err error) {
internal/guest/runtime/runc/utils.go:131:	} else if strings.Contains(s, "container with id exists") || strings.Contains(s, libcontainer.ErrExist.Error()) {
internal/guest/runtime/runc/utils.go:133:	} else if strings.Contains(s, "invalid id format") || strings.Contains(s, libcontainer.ErrInvalidID.Error()) {
internal/guest/runtime/runc/utils.go:137:	} else if strings.Contains(s, libcontainer.ErrRunning.Error()) {
internal/guest/runtime/runc/utils.go:139:	} else if strings.Contains(s, libcontainer.ErrNotRunning.Error()) {
internal/guest/runtime/runc/utils.go:147:func getRuncLogError(logPath string) error {
internal/guest/runtime/runc/utils.go:162:			lastErr = entry.asError()
internal/guest/spec/spec.go:56:	}).Info("GenerateWorkloadContainerNetworkMounts: resolved mount source root directory")
internal/guest/spec/spec.go:467:		log.G(ctx).WithField("sizeKB", val).Debug("set custom /dev/shm size")
internal/guest/spec/spec.go:479:			}).WithError(err).Warning("annotation value could not be parsed")
internal/guest/spec/spec_devices.go:74:				}).Debug("adding host devices associated with Windows device")
internal/guest/spec/spec_devices.go:82:			entry.Warn("unknown device type")
internal/guest/spec/spec_devices.go:137:				entry.WithError(err).Debugf("failed to find sysfs path for device %s", d.Path)
internal/guest/stdio/stdio.go:175:		}).Error("opengcs::PipeRelay::copyAndCleanClose - error copying from pipe")
internal/guest/stdio/stdio.go:190:			}).Error("opengcs::PipeRelay::copyAndCleanClose - error reading for clean close")
internal/guest/stdio/stdio.go:196:		}).Error("opengcs::PipeRelay::copyAndCleanClose - error shutting down socket")
internal/guest/stdio/stdio.go:202:		}).Error("opengcs::PipeRelay::copyAndCleanClose - error closing socket")
internal/guest/stdio/stdio.go:216:				}).Error("opengcs::PipeRelay::Start - error copying stdin to pipe")
internal/guest/stdio/stdio.go:221:				}).Error("opengcs::PipeRelay::Start - error closing stdin write pipe")
internal/guest/stdio/stdio.go:288:				if !strings.Contains(err.Error(), "file already closed") {
internal/guest/stdio/stdio.go:291:					}).Error("opengcs::PipeRelay::closePipes - error closing relay pipe")
internal/guest/stdio/stdio.go:339:				}).Error("opengcs::TtyRelay::Start - error copying stdin to pty")
internal/guest/stdio/stdio.go:350:				}).Error("opengcs::TtyRelay::Start - error copying pty to stdout")
internal/guest/storage/crypt/crypt.go:51:			log.G(ctx).WithError(err).WithFields(logrus.Fields{
internal/guest/storage/crypt/crypt.go:59:						log.G(ctx).WithError(err).Warning("cryptsetup failed, context timeout")
internal/guest/storage/crypt/crypt.go:157:			log.G(ctx).WithError(err).Debugf("failed to delete temporary folder: %s", tempDir)
internal/guest/storage/crypt/crypt.go:180:				log.G(ctx).WithError(inErr).Debug("failed to cleanup crypt device")
internal/guest/storage/crypt/crypt_test.go:29:func Test_Encrypt_Generate_Key_Error(t *testing.T) {
internal/guest/storage/crypt/crypt_test.go:55:func Test_Encrypt_Cryptsetup_Format_Error(t *testing.T) {
internal/guest/storage/crypt/crypt_test.go:88:func Test_Encrypt_Cryptsetup_Open_Error(t *testing.T) {
internal/guest/storage/crypt/crypt_test.go:161:func Test_Cleanup_Dm_Crypt_Error(t *testing.T) {
internal/guest/storage/devicemapper/devicemapper.go:120:func (err *dmError) Error() string {
internal/guest/storage/devicemapper/devicemapper.go:125:	return "device-mapper " + op + ": " + err.Err.Error()
internal/guest/storage/devicemapper/devicemapper.go:237:		log.G(ctx).WithError(err).Warning("CreateDevice error")
internal/guest/storage/devicemapper/devicemapper.go:248:					log.G(ctx).WithError(err).Error("CreateDeviceWithRetryErrors failed, context timeout")
internal/guest/storage/devicemapper/devicemapper_test.go:86:func TestCreateError(t *testing.T) {
internal/guest/storage/devicemapper/devicemapper_test.go:107:func TestReadOnlyError(t *testing.T) {
internal/guest/storage/devicemapper/devicemapper_test.go:128:func TestLinearError(t *testing.T) {
internal/guest/storage/devicemapper/devicemapper_test.go:194:func TestCreateDeviceWithRetryError(t *testing.T) {
internal/guest/storage/mount_test.go:54:func Test_Unmount_Stat_OtherError_Error(t *testing.T) {
internal/guest/storage/mount_test.go:121:func Test_Unmount_OtherError(t *testing.T) {
internal/guest/storage/overlay/overlay.go:36:		log.G(ctx).WithError(statErr).WithField("path", filepath.Dir(path)).Warn("failed to get disk information for ENOSPC error")
internal/guest/storage/overlay/overlay.go:56:	}).WithError(err).Warn("got ENOSPC, gathering diagnostics")
internal/guest/storage/pmem/pmem.go:47:				log.G(ctx).WithError(err).Debugf("error cleaning up target: %s", target)
internal/guest/storage/pmem/pmem.go:104:					log.G(mCtx).WithError(err).Debugf("failed to cleanup linear target: %s", dmLinearName)
internal/guest/storage/pmem/pmem.go:118:					log.G(mCtx).WithError(err).Debugf("failed to cleanup verity target: %s", dmVerityName)
internal/guest/storage/pmem/pmem.go:151:			log.G(ctx).WithError(err).Debugf("failed to remove dm verity target: %s", dmVerityName)
internal/guest/storage/pmem/pmem.go:159:			log.G(ctx).WithError(err).Debugf("failed to remove dm linear target: %s", dmLinearName)
internal/guest/storage/pmem/pmem_test.go:28:func Test_Mount_Mkdir_Fails_Error(t *testing.T) {
internal/guest/storage/scsi/scsi.go:163:						log.G(spnCtx).WithError(err).WithField("verityTarget", dmVerityName).Debug("failed to cleanup verity target")
internal/guest/storage/scsi/scsi.go:222:			log.G(ctx).WithError(err).Debug("get device filesystem failed, retrying in 500ms")
internal/guest/storage/scsi/scsi.go:230:		log.G(ctx).WithField("filesystem", deviceFS).Debug("filesystem found on device")
internal/guest/storage/scsi/scsi.go:318:			log.G(ctx).WithError(err).Debugf("failed to remove dm verity target: %s", dmVerityName)
internal/guest/storage/scsi/scsi.go:365:					log.G(ctx).WithField("blockPath", blockPath).Warn(
internal/guest/storage/scsi/scsi.go:412:	log.G(ctx).WithField("devicePath", devicePath).Debug("found device path")
internal/guest/storage/scsi/scsi.go:432:			log.G(ctx).WithError(err).Warnf("failed to close file: %s", devicePath)
internal/guest/storage/scsi/scsi_test.go:94:func (d *fakeDirEntry) Info() (os.FileInfo, error) {
internal/guest/storage/scsi/scsi_test.go:116:func Test_Mount_Mkdir_Fails_Error(t *testing.T) {
internal/guest/storage/scsi/scsi_test.go:981:func Test_Mount_EncryptDevice_Mkfs_Error(t *testing.T) {
internal/guest/storage/scsi/scsi_test.go:1155:func Test_GetDevicePath_Device_With_Partition_Error(t *testing.T) {
internal/guest/storage/scsi/scsi_test.go:1258:func Test_GetDevicePath_Device_No_Partition_Error(t *testing.T) {
internal/guest/storage/scsi/scsi_test.go:1298:func Test_GetDeviceFsType_Error(t *testing.T) {
internal/guest/transport/devnull.go:25:	}).Info("opengcs::DevNullTransport::Dial")
internal/guest/transport/log.go:23:	c.entry.Debug("opengcs::logConnection::Close - closing connection")
internal/guest/transport/log.go:29:	c.entry.Debug("opengcs::logConnection::Close - closing read connection")
internal/guest/transport/log.go:35:	c.entry.Debug("opengcs::logConnection::Close - closing write connection")
internal/guest/transport/vsock.go:28:	}).Info("opengcs::VsockTransport::Dial - vsock dial port")
internal/hcs/callback.go:156:	log.Debug("HCS notification")
internal/hcs/errors.go:136:			log.G(ctx).WithError(err).Warning("Could not unmarshal HCS result")
internal/hcs/errors.go:152:func (e *HcsError) Error() string {
internal/hcs/errors.go:153:	s := e.Op + ": " + e.Err.Error()
internal/hcs/errors.go:172:	err := e.netError()
internal/hcs/errors.go:177:	err := e.netError()
internal/hcs/errors.go:181:func (e *HcsError) netError() (err net.Error) {
internal/hcs/errors.go:196:func (e *SystemError) Error() string {
internal/hcs/errors.go:197:	s := e.Op + " " + e.ID + ": " + e.Err.Error()
internal/hcs/errors.go:204:func makeSystemError(system *System, op string, err error, events []ErrorEvent) error {
internal/hcs/errors.go:230:func (e *ProcessError) Error() string {
internal/hcs/errors.go:231:	s := fmt.Sprintf("%s %s:%d: %s", e.Op, e.SystemID, e.Pid, e.Err.Error())
internal/hcs/errors.go:238:func makeProcessError(process *Process, op string, err error, events []ErrorEvent) error {
internal/hcs/errors_test.go:16:func (e *MyError) Error() string {
internal/hcs/errors_test.go:107:func (e netError) Error() string   { return "temporary timeout" }
internal/hcs/process.go:77:					log.G(ctx).WithError(err).Warn("force unblocking process waits")
internal/hcs/process.go:101:		return false, makeProcessError(process, operation, ErrAlreadyClosed, nil)
internal/hcs/process.go:113:		err = makeProcessError(process, operation, err, events)
internal/hcs/process.go:126:		return false, makeProcessError(process, operation, ErrAlreadyClosed, nil)
internal/hcs/process.go:130:		return false, makeProcessError(process, operation, ErrProcessAlreadyStopped, nil)
internal/hcs/process.go:150:		log.G(ctx).WithField("err", err).Error("OpenComputeSystem() call failed")
internal/hcs/process.go:154:			log.G(ctx).WithField("err", err).Error("Terminate() call failed")
internal/hcs/process.go:200:		err = makeProcessError(newProcessHandle, operation, err, events)
internal/hcs/process.go:229:		err = makeProcessError(process, operation, err, nil)
internal/hcs/process.go:230:		log.G(ctx).WithError(err).Error("failed wait")
internal/hcs/process.go:240:				err = makeProcessError(process, operation, err, events)
internal/hcs/process.go:245:					err = makeProcessError(process, operation, err, nil)
internal/hcs/process.go:256:	log.G(ctx).WithField("exitCode", exitCode).Debug("process exited")
internal/hcs/process.go:291:		return makeProcessError(process, operation, ErrAlreadyClosed, nil)
internal/hcs/process.go:309:		return makeProcessError(process, operation, err, events)
internal/hcs/process.go:319:		return -1, makeProcessError(process, "hcs::Process::ExitCode", ErrInvalidProcessState, nil)
internal/hcs/process.go:343:		return nil, nil, nil, makeProcessError(process, operation, ErrAlreadyClosed, nil)
internal/hcs/process.go:355:	processInfo, resultJSON, err := vmcompute.HcsGetProcessInfo(ctx, process.handle)
internal/hcs/process.go:358:		return nil, nil, nil, makeProcessError(process, operation, err, events)
internal/hcs/process.go:363:		return nil, nil, nil, makeProcessError(process, operation, err, nil)
internal/hcs/process.go:392:		return makeProcessError(process, operation, ErrAlreadyClosed, nil)
internal/hcs/process.go:412:			return makeProcessError(process, operation, err, events)
internal/hcs/process.go:509:		return makeProcessError(process, operation, err, nil)
internal/hcs/process.go:513:		return makeProcessError(process, operation, err, nil)
internal/hcs/system.go:94:			return nil, makeSystemError(computeSystem, operation, err, nil)
internal/hcs/system.go:106:		return nil, makeSystemError(computeSystem, operation, err, events)
internal/hcs/system.go:123:		return nil, makeSystemError(computeSystem, operation, err, events)
internal/hcs/system.go:132:		return nil, makeSystemError(computeSystem, operation, err, nil)
internal/hcs/system.go:211:		return makeSystemError(computeSystem, operation, ErrAlreadyClosed, nil)
internal/hcs/system.go:218:		return makeSystemError(computeSystem, operation, err, events)
internal/hcs/system.go:246:		return makeSystemError(computeSystem, operation, err, events)
internal/hcs/system.go:268:		return makeSystemError(computeSystem, operation, err, events)
internal/hcs/system.go:286:		log.G(ctx).Debug("system exited")
internal/hcs/system.go:288:		log.G(ctx).Debug("unexpected system exit")
internal/hcs/system.go:289:		computeSystem.exitError = makeSystemError(computeSystem, operation, err, nil)
internal/hcs/system.go:292:		err = makeSystemError(computeSystem, operation, err, nil)
internal/hcs/system.go:306:func (computeSystem *System) WaitError() error {
internal/hcs/system.go:322:		return computeSystem.WaitError()
internal/hcs/system.go:339:func (computeSystem *System) ExitError() error {
internal/hcs/system.go:357:		return nil, makeSystemError(computeSystem, operation, ErrAlreadyClosed, nil)
internal/hcs/system.go:362:		return nil, makeSystemError(computeSystem, operation, err, nil)
internal/hcs/system.go:368:		return nil, makeSystemError(computeSystem, operation, err, events)
internal/hcs/system.go:376:		return nil, makeSystemError(computeSystem, operation, err, nil)
internal/hcs/system.go:414:					log.G(ctx).WithError(err).Warn("failed to get statistics in-proc")
internal/hcs/system.go:498:		return nil, makeSystemError(computeSystem, operation, ErrAlreadyClosed, nil)
internal/hcs/system.go:503:		return nil, makeSystemError(computeSystem, operation, err, nil)
internal/hcs/system.go:509:		return nil, makeSystemError(computeSystem, operation, err, events)
internal/hcs/system.go:517:		return nil, makeSystemError(computeSystem, operation, err, nil)
internal/hcs/system.go:549:		logEntry = logEntry.WithError(fmt.Errorf("failed to query compute system properties in-proc: %w", err))
internal/hcs/system.go:556:	}).Info("falling back to HCS for property type queries")
internal/hcs/system.go:592:		return makeSystemError(computeSystem, operation, ErrAlreadyClosed, nil)
internal/hcs/system.go:599:		return makeSystemError(computeSystem, operation, err, events)
internal/hcs/system.go:620:		return makeSystemError(computeSystem, operation, ErrAlreadyClosed, nil)
internal/hcs/system.go:627:		return makeSystemError(computeSystem, operation, err, events)
internal/hcs/system.go:653:		return makeSystemError(computeSystem, operation, ErrAlreadyClosed, nil)
internal/hcs/system.go:660:		return makeSystemError(computeSystem, operation, err, events)
internal/hcs/system.go:671:		return nil, nil, makeSystemError(computeSystem, operation, ErrAlreadyClosed, nil)
internal/hcs/system.go:676:		return nil, nil, makeSystemError(computeSystem, operation, err, nil)
internal/hcs/system.go:688:		return nil, nil, makeSystemError(computeSystem, operation, err, events)
internal/hcs/system.go:691:	log.G(ctx).WithField("pid", processInfo.ProcessId).Debug("created process pid")
internal/hcs/system.go:710:		return nil, makeSystemError(computeSystem, operation, err, nil)
internal/hcs/system.go:718:		return nil, makeSystemError(computeSystem, operation, err, nil)
internal/hcs/system.go:733:		return nil, makeSystemError(computeSystem, operation, ErrAlreadyClosed, nil)
internal/hcs/system.go:739:		return nil, makeSystemError(computeSystem, operation, err, events)
internal/hcs/system.go:744:		return nil, makeSystemError(computeSystem, operation, err, nil)
internal/hcs/system.go:776:		return makeSystemError(computeSystem, operation, err, nil)
internal/hcs/system.go:781:		return makeSystemError(computeSystem, operation, err, nil)
internal/hcs/system.go:859:		return makeSystemError(computeSystem, operation, ErrAlreadyClosed, nil)
internal/hcs/system.go:871:		return makeSystemError(computeSystem, operation, err, events)
internal/hcs/waithelper.go:37:		log.G(ctx).WithField("callbackNumber", callbackNumber).Error("failed to waitForNotification: callbackNumber does not exist in callbackMap")
internal/hcs/waithelper.go:45:		log.G(ctx).WithField("type", expectedNotification).Error("unknown notification type in waitForNotification")
internal/hcserror/hcserror.go:18:func (e *HcsError) Error() string {
internal/hcserror/hcserror.go:23:	s += fmt.Sprintf("failed in Win32: %s (0x%x)", e.Err, Win32FromError(e.Err))
internal/hcserror/hcserror.go:42:func Win32FromError(err error) uint32 {
internal/hcserror/hcserror.go:45:		return Win32FromError(herr.Err)
internal/hcsoci/create.go:134:	}).Debug("hcsshim::initializeCreateOptions")
internal/hcsoci/create.go:244:	log.G(ctx).Debug("hcsshim::CreateContainer allocating resources")
internal/hcsoci/create.go:249:		log.G(ctx).Debug("hcsshim::CreateContainer allocateLinuxResources")
internal/hcsoci/create.go:252:			log.G(ctx).WithError(err).Debug("failed to allocateLinuxResources")
internal/hcsoci/create.go:257:			log.G(ctx).WithError(err).Debug("failed createHCSContainerDocument")
internal/hcsoci/create.go:263:			log.G(ctx).WithError(err).Debug("failed to allocateWindowsResources")
internal/hcsoci/create.go:266:		log.G(ctx).Debug("hcsshim::CreateContainer creating container document")
internal/hcsoci/create.go:269:			log.G(ctx).WithError(err).Debug("failed createHCSContainerDocument")
internal/hcsoci/create.go:304:	log.G(ctx).Debug("hcsshim::CreateContainer creating compute system")
internal/hcsoci/create.go:312:			log.G(ctx).Debug("redirecting container HvSocket for WCOW")
internal/hcsoci/create.go:321:			addressInfoCloser, err := hvsocket.CreateContainerAddressInfo(containerSystemGUID, coi.HostingSystem.RuntimeID())
internal/hcsoci/devices.go:110:					log.G(ctx).WithError(releaseErr).Error("failed to release container resource")
internal/hcsoci/devices.go:144:			log.G(ctx).WithField("parsed devices", specDev).Info("added windows device to spec")
internal/hcsoci/devices.go:167:					log.G(ctx).WithError(releaseErr).Error("failed to release container resource")
internal/hcsoci/devices.go:204:					log.G(ctx).WithError(releaseErr).Error("failed to release container resource")
internal/hcsoci/hcsdoc_lcow.go:110:	log.G(ctx).WithField("guestRoot", guestRoot).Debug("hcsshim::createLinuxContainerDoc")
internal/hcsoci/hcsdoc_wcow.go:143:	log.G(ctx).Debug("hcsshim: CreateHCSContainerDocument")
internal/hcsoci/hcsdoc_wcow.go:227:		}).Info("rescaling CPU limit for UVM sandbox")
internal/hcsoci/hcsdoc_wcow.go:250:			}).Debug("container memory size limit exceeds maximum value allowed in v1 HCS schema")
internal/hcsoci/hcsdoc_wcow.go:493:		log.G(ctx).WithField("count", len(testAnnotationValues)).Info("adding test annotation registry values to container")
internal/hcsoci/hcsdoc_wcow.go:496:		log.G(ctx).Debug("no test annotation registry values found in container annotations")
internal/hcsoci/hcsdoc_wcow.go:526:		log.G(ctx).WithField("hcsv2 device", v2Dev).Debug("adding assigned device to container doc")
internal/hcsoci/network.go:19:	l.Debug(op + " - Begin")
internal/hcsoci/network.go:21:		l.Debug(op + " - End")
internal/hcsoci/network.go:32:	}).Info("created network namespace for container")
internal/hcsoci/network.go:46:		}).Info("added network endpoint to namespace")
internal/hcsoci/resources.go:29:		}).Warn("Changing user requested cpu count to current number of processors on the host")
internal/hcsoci/resources.go:44:		}).Warn("Changing user requested MemorySizeInMB to align to 2MB")
internal/hcsoci/resources_lcow.go:32:		log.G(ctx).Debug("hcsshim::allocateLinuxResources mounting storage")
internal/hcsoci/resources_lcow.go:90:				l.Debug("hcsshim::allocateLinuxResources Hot-adding SCSI physical disk for OCI mount")
internal/hcsoci/resources_lcow.go:110:				l.Debug("hcsshim::allocateLinuxResources Hot-adding SCSI virtual disk for OCI mount")
internal/hcsoci/resources_lcow.go:134:				l.Debug("hcsshim::allocateLinuxResources Hot-adding ExtensbleVirtualDisk")
internal/hcsoci/resources_lcow.go:194:				l.Debug("hcsshim::allocateLinuxResources Hot-adding Plan9 for OCI mount")
internal/hcsoci/resources_wcow.go:39:			log.G(ctx).Debug("hcsshim::allocateWindowsResources mounting storage")
internal/hcsoci/resources_wcow.go:157:				l.Debug("hcsshim::allocateWindowsResources Hot-adding SCSI physical disk for OCI mount")
internal/hcsoci/resources_wcow.go:167:				l.Debug("hcsshim::allocateWindowsResources Hot-adding SCSI virtual disk for OCI mount")
internal/hcsoci/resources_wcow.go:177:				l.Debug("hcsshim::allocateWindowsResource Hot-adding ExtensibleVirtualDisk")
internal/hcsoci/resources_wcow.go:236:			l.Debug("hcsshim::allocateWindowsResources Hot-adding VSMB share for OCI mount")
internal/hns/hns.go:13:func (e EndpointNotFoundError) Error() string {
internal/hns/hns.go:21:func (e NetworkNotFoundError) Error() string {
internal/hns/hnsaccelnet.go:50:	logrus.Debug(title)
internal/hns/hnsaccelnet.go:58:	logrus.Debug(title)
internal/hns/namespace.go:56:		if strings.Contains(err.Error(), "Element not found.") {
internal/hvsocket/hvsocket.go:42:func CreateContainerAddressInfo(containerID, uvmID guid.GUID) (resources.ResourceCloser, error) {
internal/hvsocket/hvsocket.go:43:	return CreateAddressInfo(containerID, uvmID, guid.GUID{}, true)
internal/hvsocket/hvsocket.go:56:func CreateAddressInfo(systemID, vmID, siloID guid.GUID, passthru bool) (resources.ResourceCloser, error) {
internal/hvsocket/hvsocket.go:97:			log.G(context.Background()).WithError(closeErr).Debug("failed to close address info handle")
internal/jobcontainers/jobcontainer.go:102:	log.G(ctx).WithField("id", id).Debug("Creating job container")
internal/jobcontainers/jobcontainer.go:441:		log.G(context.Background()).WithError(err).WithField("cid", c.id).Warning("failed to close job object")
internal/jobcontainers/jobcontainer.go:446:		log.G(context.Background()).WithError(err).WithField("cid", c.id).Warning("failed to close token")
internal/jobcontainers/jobcontainer.go:453:			log.G(context.Background()).WithError(err).WithField("cid", c.id).Warning("failed to delete local account")
internal/jobcontainers/jobcontainer.go:475:	log.G(ctx).WithField("id", c.id).Debug("shutting down job container")
internal/jobcontainers/jobcontainer.go:499:			log.G(ctx).WithField("pid", pid).Error("failed to signal process in job container")
internal/jobcontainers/jobcontainer.go:582:	err := forEachProcessInfo(c.job, func(procInfo *winapi.SYSTEM_PROCESS_INFORMATION) {
internal/jobcontainers/jobcontainer.go:604:	log.G(ctx).WithField("id", c.id).Debug("terminating job container")
internal/jobcontainers/jobcontainer.go:616:func (c *JobContainer) WaitError() error {
internal/jobcontainers/jobcontainer.go:624:	return c.WaitError()
internal/jobcontainers/jobcontainer.go:657:			log.G(ctx).WithError(err).Warn("error while polling for job container notification")
internal/jobcontainers/jobcontainer.go:667:			log.G(ctx).WithField("message", msg).Warn("unknown job object notification encountered")
internal/jobcontainers/jobcontainer.go:684:func forEachProcessInfo(job *jobobject.JobObject, work func(*winapi.SYSTEM_PROCESS_INFORMATION)) error {
internal/jobcontainers/jobcontainer.go:733:			return nil, winapi.RtlNtStatusToDosError(status)
internal/jobcontainers/logon.go:28:	if err := winapi.NetLocalGroupGetInfo(
internal/jobcontainers/logon.go:77:	log.G(ctx).WithField("username", user).Debug("Created local user account for job container")
internal/jobcontainers/mounts.go:108:			}).Warn("job container mount destination exists and will be shadowed")
internal/jobcontainers/mounts.go:133:			log.G(ctx).WithError(err).Warnf("failed to setup symlink from %s to containers rootfs at %s", mount.Source, fullCtrPath)
internal/jobcontainers/oci_test.go:53:	if !strings.Contains(err.Error(), "multiple processor groups") {
internal/jobcontainers/oci_test.go:75:	if !strings.Contains(err.Error(), "processor group") {
internal/jobcontainers/oci_test.go:97:	if !strings.Contains(err.Error(), "mask must be non-zero") {
internal/jobcontainers/process.go:161:	log.G(ctx).WithField("pid", p.Pid()).Debug("waitBackground for JobProcess")
internal/jobcontainers/process.go:231:	log.G(ctx).WithField("pid", p.Pid()).Debug("killing job process")
internal/jobcontainers/storage.go:45:		log.G(ctx).Debug("mounting job container storage")
internal/jobcontainers/storage.go:54:					log.G(ctx).WithError(closeErr).Errorf("failed to cleanup mounted layers during another failure(%s)", err)
internal/jobcontainers/system_test.go:9:func TestSystemInfo(t *testing.T) {
internal/jobobject/iocp.go:46:			log.G(ctx).WithError(err).Error("failed to poll for job object message")
internal/jobobject/iocp.go:52:				log.G(ctx).WithField("value", msq).Warn("encountered non queue type in job map")
internal/jobobject/iocp.go:60:				}).Warn("failed to parse job object message")
internal/jobobject/iocp.go:72:				}).Warn("tried to write to a closed queue")
internal/jobobject/iocp.go:76:			log.G(ctx).Warn("received a message for a job not present in the mapping")
internal/jobobject/jobobject.go:111:			return nil, winapi.RtlNtStatusToDosError(status)
internal/jobobject/jobobject.go:188:			return nil, winapi.RtlNtStatusToDosError(status)
internal/jobobject/jobobject.go:616:			return 0, fmt.Errorf("failed to query information for process: %w", winapi.RtlNtStatusToDosError(status))
internal/jobobject/limits.go:114:	return job.setCPURateControlInfo(cpuInfo)
internal/jobobject/limits.go:187:	return job.setIORateControlInfo(ioInfo)
internal/jobobject/limits.go:300:func (job *JobObject) setIORateControlInfo(ioInfo *winapi.JOBOBJECT_IO_RATE_CONTROL_INFORMATION) error {
internal/jobobject/limits.go:315:func (job *JobObject) setCPURateControlInfo(cpuInfo *winapi.JOBOBJECT_CPU_RATE_CONTROL_INFORMATION) error {
internal/layers/lcow.go:51:		log.G(ctx).WithError(err).Error("failed RemoveCombinedLayersLCOW")
internal/layers/lcow.go:57:		log.G(ctx).WithError(err).Error("failed LCOW scratch mount release")
internal/layers/lcow.go:67:			}).Error("failed releasing LCOW layer")
internal/layers/lcow.go:97:	log.G(ctx).WithField("os", vm.OS()).Debug("hcsshim::MountLCOWLayers V2 UVM")
internal/layers/lcow.go:107:					log.G(ctx).WithError(err).Warn("failed to remove lcow layer on cleanup")
internal/layers/lcow.go:114:		log.G(ctx).WithField("layerPath", layer.VHDPath).Debug("mounting layer")
internal/layers/lcow.go:134:	log.G(ctx).WithField("hostPath", hostPath).Debug("mounting scratch VHD")
internal/layers/lcow.go:169:				log.G(ctx).WithError(err).Warn("failed to remove scratch on cleanup")
internal/layers/lcow.go:179:	log.G(ctx).Debug("hcsshim::MountLCOWLayers Succeeded")
internal/layers/lcow.go:201:			}).Debug("Added LCOW layer")
internal/layers/lcow.go:226:	}).Debug("Added LCOW layer")
internal/layers/wcow_mount.go:165:					log.G(ctx).WithField("path", l.scratchLayerPath).WithError(hcserr.Err).Warning("retrying layer operations after failure")
internal/layers/wcow_mount.go:236:				log.G(ctx).WithError(err).Warnf("mount process isolated cim layers common, undo failed with: %s", rErr)
internal/layers/wcow_mount.go:255:	}).Debug("scratch activated")
internal/layers/wcow_mount.go:281:	log.G(ctx).WithField("layer data", layerData).Debug("unionFS filter attached")
internal/layers/wcow_mount.go:303:				log.G(ctx).WithError(err).Warnf("mount process isolated forked CIM layers, undo failed with: %s", rErr)
internal/layers/wcow_mount.go:336:				log.G(ctx).WithError(err).Warnf("mount process isolated forked CIM layers, undo failed with: %s", rErr)
internal/layers/wcow_mount.go:345:	}).Debug("mounting process isolated block CIM layers")
internal/layers/wcow_mount.go:359:	log.G(ctx).WithField("volume", volume).Debug("mounted blockCIM layers for process isolated container")
internal/layers/wcow_mount.go:379:		log.G(ctx).WithError(err).Error("failed RemoveCombinedLayersWCOW")
internal/layers/wcow_mount.go:385:		log.G(ctx).WithError(err).Error("failed WCOW scratch mount release")
internal/layers/wcow_mount.go:395:			}).Error("failed releasing WCOW layer")
internal/layers/wcow_mount.go:405:	log.G(ctx).WithField("os", vm.OS()).Debug("hcsshim::MountWCOWLayers V2 UVM")
internal/layers/wcow_mount.go:421:					log.G(ctx).WithError(err).Warn("failed to remove wcow layer on cleanup")
internal/layers/wcow_mount.go:428:		log.G(ctx).WithField("layerPath", layerPath).Debug("mounting layer")
internal/layers/wcow_mount.go:440:	log.G(ctx).WithField("hostPath", hostPath).Debug("mounting scratch VHD")
internal/layers/wcow_mount.go:451:				log.G(ctx).WithError(err).Warn("failed to remove scratch on cleanup")
internal/layers/wcow_mount.go:486:	log.G(ctx).Debug("hcsshim::MountWCOWLayers Succeeded")
internal/layers/wcow_mount.go:507:				log.G(ctx).WithError(err).Warnf("mount process isolated forked CIM layers, undo failed with: %s", rErr)
internal/layers/wcow_mount.go:516:	}).Debug("mounting hyperv isolated block CIM layers")
internal/layers/wcow_mount.go:525:	log.G(ctx).WithField("volume", mountedCIMs.MountedVolumePath()).Debug("mounted blockCIM layers for hyperV isolated container")
internal/layers/wcow_mount.go:542:	}).Debug("mounted scratch VHD")
internal/layers/wcow_mount.go:570:	log.G(ctx).Debug("hcsshim::mountHyperVIsolatedBlockCIMLayers Succeeded")
internal/layers/wcow_mount.go:585:	}).Debug("mounting volume for container")
internal/layers/wcow_mount.go:614:	}).Debug("removing volume mount point for container")
internal/lcow/common.go:51:	}).Debug("lcow::FormatDisk device guest location")
internal/lcow/common.go:64:	log.G(ctx).WithField("dest", destPath).Debug("lcow::FormatDisk complete")
internal/lcow/disk.go:31:	}).Debug("lcow::FormatDisk opts")
internal/lcow/disk.go:47:	}).Debug("lcow::FormatDisk device attached")
internal/lcow/disk.go:52:	log.G(ctx).WithField("dest", destPath).Debug("lcow::FormatDisk complete")
internal/lcow/scratch.go:51:	}).Debug("lcow::CreateScratch opts")
internal/lcow/scratch.go:62:			}).Debug("lcow::CreateScratch copied from cache")
internal/lcow/scratch.go:98:	}).Debug("lcow::CreateScratch device attached")
internal/lcow/scratch.go:108:		log.G(ctx).WithError(err).WithField("stderr", mkfsStderr.String()).Error("mkfs.ext4 failed")
internal/lcow/scratch.go:125:	log.G(ctx).WithField("dest", destFile).Debug("lcow::CreateScratch created (non-cache)")
internal/log/format.go:72:		}).Debug("could not format value")
internal/log/hook.go:153:			d[k+"-"+logrus.ErrorKey] = err.Error()
internal/ncproxyttrpc/networkconfigproxy.pb.go:82:	ms.StoreMessageInfo(mi)
internal/ncproxyttrpc/networkconfigproxy.pb.go:95:		if ms.LoadMessageInfo() == nil {
internal/ncproxyttrpc/networkconfigproxy.pb.go:96:			ms.StoreMessageInfo(mi)
internal/ncproxyttrpc/networkconfigproxy.pb.go:132:	ms.StoreMessageInfo(mi)
internal/ncproxyttrpc/networkconfigproxy.pb.go:145:		if ms.LoadMessageInfo() == nil {
internal/ncproxyttrpc/networkconfigproxy.pb.go:146:			ms.StoreMessageInfo(mi)
internal/ncproxyttrpc/networkconfigproxy.pb.go:169:	ms.StoreMessageInfo(mi)
internal/ncproxyttrpc/networkconfigproxy.pb.go:182:		if ms.LoadMessageInfo() == nil {
internal/ncproxyttrpc/networkconfigproxy.pb.go:183:			ms.StoreMessageInfo(mi)
internal/ncproxyttrpc/networkconfigproxy.pb.go:212:	ms.StoreMessageInfo(mi)
internal/ncproxyttrpc/networkconfigproxy.pb.go:225:		if ms.LoadMessageInfo() == nil {
internal/ncproxyttrpc/networkconfigproxy.pb.go:226:			ms.StoreMessageInfo(mi)
internal/ncproxyttrpc/networkconfigproxy.pb.go:250:	ms.StoreMessageInfo(mi)
internal/ncproxyttrpc/networkconfigproxy.pb.go:263:		if ms.LoadMessageInfo() == nil {
internal/ncproxyttrpc/networkconfigproxy.pb.go:264:			ms.StoreMessageInfo(mi)
internal/ncproxyttrpc/networkconfigproxy.pb.go:300:	ms.StoreMessageInfo(mi)
internal/ncproxyttrpc/networkconfigproxy.pb.go:313:		if ms.LoadMessageInfo() == nil {
internal/ncproxyttrpc/networkconfigproxy.pb.go:314:			ms.StoreMessageInfo(mi)
internal/oc/errors.go:21:	if s, ok := status.FromError(errdefs.ToGRPC(err)); ok {
internal/oc/span.go:18:		status.Message = err.Error()
internal/oci/annotations.go:51:					}).WithError(err).Warning("annotation expansion would overwrite conflicting value")
internal/oci/annotations.go:74:		}).WithError(err).Warning("Host process container and disable host process container cannot both be true")
internal/oci/annotations.go:111:		logAnnotationValueParseError(ctx, k, v, fmt.Sprintf("%T", t), err)
internal/oci/annotations.go:238:			entry.WithError(err).Warn("invalid GUID string for Hyper-V socket service configuration annotation")
internal/oci/annotations.go:245:			logAnnotationValueParseError(ctx, k, v, fmt.Sprintf("%T", conf), err)
internal/oci/annotations.go:252:			}).Warn("overwritting existing Hyper-V socket service configuration")
internal/oci/annotations.go:274:		logAnnotationValueParseError(ctx, key, v, logfields.Bool, err)
internal/oci/annotations.go:289:		logAnnotationValueParseError(ctx, key, v, logfields.Bool, err)
internal/oci/annotations.go:303:		logAnnotationValueParseError(ctx, key, v, logfields.Int32, err)
internal/oci/annotations.go:317:		logAnnotationValueParseError(ctx, key, v, logfields.Uint32, err)
internal/oci/annotations.go:330:		logAnnotationValueParseError(ctx, key, v, logfields.Uint64, err)
internal/oci/annotations.go:365:			logAnnotationValueParseError(ctx, key, cs, logfields.Uint64, err)
internal/oci/annotations.go:404:func logAnnotationValueParseError(ctx context.Context, k, v, et string, err error) {
internal/oci/annotations.go:411:		entry = entry.WithError(err)
internal/oci/uvm.go:143:			}).Warn("annotation value must be 'initrd' or 'vhd'")
internal/processorinfo/host_information.go:18:func HostProcessorInfo(ctx context.Context) (*hcsschema.ProcessorTopology, error) {
internal/regopolicyinterpreter/regopolicyinterpreter.go:338:	r.logInfo("Logging Enabled at level %d", level)
internal/regopolicyinterpreter/regopolicyinterpreter.go:363:		r.logInfo("Logging disabled")
internal/regopolicyinterpreter/regopolicyinterpreter.go:409:func (r *RegoPolicyInterpreter) logInfo(message string, args ...interface{}) {
internal/regopolicyinterpreter/regopolicyinterpreter.go:423:		r.resultsLogger.Printf("error marshaling result set: %v\n", err.Error())
internal/regopolicyinterpreter/regopolicyinterpreter.go:436:		r.metadataLogger.Printf("error marshaling metadata: %v\n", err.Error())
internal/regopolicyinterpreter/regopolicyinterpreter.go:560:	r.logInfo("%s", output)
internal/regopolicyinterpreter/regopolicyinterpreter_test.go:47:			t.Error(err)
internal/regopolicyinterpreter/regopolicyinterpreter_test.go:63:			t.Error(err)
internal/regopolicyinterpreter/regopolicyinterpreter_test.go:87:			t.Error(err)
internal/regopolicyinterpreter/regopolicyinterpreter_test.go:94:			t.Error(err)
internal/regopolicyinterpreter/regopolicyinterpreter_test.go:121:			t.Error(err)
internal/regopolicyinterpreter/regopolicyinterpreter_test.go:161:			t.Error("received empty result from query")
internal/regopolicyinterpreter/regopolicyinterpreter_test.go:201:			t.Error("received empty result from query")
internal/regopolicyinterpreter/regopolicyinterpreter_test.go:209:			t.Error(err)
internal/regopolicyinterpreter/regopolicyinterpreter_test.go:235:			t.Error(err)
internal/regopolicyinterpreter/regopolicyinterpreter_test.go:251:			t.Error(err)
internal/regopolicyinterpreter/regopolicyinterpreter_test.go:257:			t.Error(err)
internal/regopolicyinterpreter/regopolicyinterpreter_test.go:288:				t.Error(err)
internal/regopolicyinterpreter/regopolicyinterpreter_test.go:301:			t.Error(err)
internal/regopolicyinterpreter/regopolicyinterpreter_test.go:307:			t.Error(err)
internal/regopolicyinterpreter/regopolicyinterpreter_test.go:336:				t.Error(err)
internal/regopolicyinterpreter/regopolicyinterpreter_test.go:349:			t.Error(err)
internal/regopolicyinterpreter/regopolicyinterpreter_test.go:385:			t.Error(err)
internal/regstate/regstate.go:43:func (err *NotFoundError) Error() string {
internal/regstate/regstate.go:47:func IsNotFoundError(err error) bool {
internal/regstate/regstate.go:57:func (err *NoStateError) Error() string {
internal/resources/resources.go:121:				log.G(ctx).Warn(err)
internal/resources/resources.go:137:					log.G(ctx).WithError(err).Error("failed to release container resource")
internal/resources/resources.go:144:				log.G(ctx).WithError(err).Error("failed to release container resource")
internal/resources/resources.go:153:					log.G(ctx).WithError(err).Error("failed to release container resource")
internal/safefile/safeopen.go:100:		return nil, winapi.RtlNtStatusToDosError(status)
internal/safefile/safeopen.go:156:		fi, err := winio.GetFileBasicInfo(parent)
internal/safefile/safeopen.go:161:			return &os.LinkError{Op: "link", Old: oldf.Name(), New: filepath.Join(newroot.Name(), newname), Err: winapi.RtlNtStatusToDosError(winapi.STATUS_REPARSE_POINT_ENCOUNTERED)}
internal/safefile/safeopen.go:194:		return &os.LinkError{Op: "link", Old: oldf.Name(), New: filepath.Join(parent.Name(), newbase), Err: winapi.RtlNtStatusToDosError(status)}
internal/safefile/safeopen.go:212:		return winapi.RtlNtStatusToDosError(status)
internal/safefile/safeopen.go:219:	bi, err := winio.GetFileBasicInfo(f)
internal/safefile/safeopen.go:232:	return winio.SetFileBasicInfo(f, &sbi)
internal/safefile/safeopen_test.go:44:	err = winio.SetFileBasicInfo(f, &bi)
internal/schemaversion/schemaversion.go:98:			logrus.WithField("schemaVersion", requestedSV).Warn("Ignoring unsupported requested schema version")
internal/security/grantvmgroupaccess.go:107:	if err := getSecurityInfo(fd, uint32(ot), uint32(si), nil, nil, &origDACL, nil, &sd); err != nil {
internal/security/grantvmgroupaccess.go:125:	if err := setSecurityInfo(fd, uint32(ot), uint32(si), uintptr(0), uintptr(0), newDACL, uintptr(0)); err != nil {
internal/security/syscall_windows.go:5://sys getSecurityInfo(handle syscall.Handle, objectType uint32, si uint32, ppsidOwner **uintptr, ppsidGroup **uintptr, ppDacl *uintptr, ppSacl *uintptr, ppSecurityDescriptor *uintptr) (win32err error) = advapi32.GetSecurityInfo
internal/security/syscall_windows.go:6://sys setSecurityInfo(handle syscall.Handle, objectType uint32, si uint32, psidOwner uintptr, psidGroup uintptr, pDacl uintptr, pSacl uintptr) (win32err error) = advapi32.SetSecurityInfo
internal/security/zsyscall_windows.go:47:func getSecurityInfo(handle syscall.Handle, objectType uint32, si uint32, ppsidOwner **uintptr, ppsidGroup **uintptr, ppDacl *uintptr, ppSacl *uintptr, ppSecurityDescriptor *uintptr) (win32err error) {
internal/security/zsyscall_windows.go:63:func setSecurityInfo(handle syscall.Handle, objectType uint32, si uint32, psidOwner uintptr, psidGroup uintptr, pDacl uintptr, pSacl uintptr) (win32err error) {
internal/shim/publisher.go:109:			log.L.WithError(err).Error("forward event")
internal/shim/shim.go:84:	Info(ctx context.Context, optionsR io.Reader) (*types.RuntimeInfo, error)
internal/shim/shim.go:206:func runInfo(ctx context.Context, manager Manager) error {
internal/shim/shim.go:207:	info, err := manager.Info(ctx, os.Stdin)
internal/shim/shim.go:231:		return runInfo(ctx, manager)
internal/shim/shim.go:354:		log.G(ctx).WithFields(log.Fields{"id": pID, "type": p.Type}).Debug("loading plugin")
internal/shim/shim.go:388:				log.G(ctx).WithFields(log.Fields{"id": pID, "type": p.Type, "error": err}).Info("skip loading plugin")
internal/shim/shim.go:395:			log.G(ctx).WithField("id", pID).Debug("registering ttrpc service")
internal/shim/shim.go:466:			log.G(ctx).WithError(err).Fatal("containerd-shim: ttrpc server failure")
internal/shim/shim.go:481:			log.G(ctx).WithError(err).Warn("Could not setup pprof")
internal/shim/shim.go:527:			log.G(ctx).WithError(err).Fatal("containerd-shim: pprof endpoint failure")
internal/shim/shim_windows.go:115:	log.L.WithField("pipe", path).Debug("serving api on named pipe")
internal/shim/shim_windows.go:122:	logger.Debug("starting signal loop")
internal/shim/shim_windows.go:129:			logger.WithField("signal", s).Debug("received signal in reap loop")
internal/shim/shim_windows.go:146:			logger.WithField("signal", s).Debug("caught exit signal")
internal/shim/shim_windows_test.go:369:		t.Error(err)
internal/shimdiag/shimdiag.pb.go:40:	ms.StoreMessageInfo(mi)
internal/shimdiag/shimdiag.pb.go:53:		if ms.LoadMessageInfo() == nil {
internal/shimdiag/shimdiag.pb.go:54:			ms.StoreMessageInfo(mi)
internal/shimdiag/shimdiag.pb.go:119:	ms.StoreMessageInfo(mi)
internal/shimdiag/shimdiag.pb.go:132:		if ms.LoadMessageInfo() == nil {
internal/shimdiag/shimdiag.pb.go:133:			ms.StoreMessageInfo(mi)
internal/shimdiag/shimdiag.pb.go:162:	ms.StoreMessageInfo(mi)
internal/shimdiag/shimdiag.pb.go:175:		if ms.LoadMessageInfo() == nil {
internal/shimdiag/shimdiag.pb.go:176:			ms.StoreMessageInfo(mi)
internal/shimdiag/shimdiag.pb.go:200:	ms.StoreMessageInfo(mi)
internal/shimdiag/shimdiag.pb.go:213:		if ms.LoadMessageInfo() == nil {
internal/shimdiag/shimdiag.pb.go:214:			ms.StoreMessageInfo(mi)
internal/shimdiag/shimdiag.pb.go:253:	ms.StoreMessageInfo(mi)
internal/shimdiag/shimdiag.pb.go:266:		if ms.LoadMessageInfo() == nil {
internal/shimdiag/shimdiag.pb.go:267:			ms.StoreMessageInfo(mi)
internal/shimdiag/shimdiag.pb.go:310:	ms.StoreMessageInfo(mi)
internal/shimdiag/shimdiag.pb.go:323:		if ms.LoadMessageInfo() == nil {
internal/shimdiag/shimdiag.pb.go:324:			ms.StoreMessageInfo(mi)
internal/shimdiag/shimdiag.pb.go:346:	ms.StoreMessageInfo(mi)
internal/shimdiag/shimdiag.pb.go:359:		if ms.LoadMessageInfo() == nil {
internal/shimdiag/shimdiag.pb.go:360:			ms.StoreMessageInfo(mi)
internal/shimdiag/shimdiag.pb.go:383:	ms.StoreMessageInfo(mi)
internal/shimdiag/shimdiag.pb.go:396:		if ms.LoadMessageInfo() == nil {
internal/shimdiag/shimdiag.pb.go:397:			ms.StoreMessageInfo(mi)
internal/shimdiag/shimdiag.pb.go:427:	ms.StoreMessageInfo(mi)
internal/shimdiag/shimdiag.pb.go:440:		if ms.LoadMessageInfo() == nil {
internal/shimdiag/shimdiag.pb.go:441:			ms.StoreMessageInfo(mi)
internal/shimdiag/shimdiag.pb.go:472:	ms.StoreMessageInfo(mi)
internal/shimdiag/shimdiag.pb.go:485:		if ms.LoadMessageInfo() == nil {
internal/shimdiag/shimdiag.pb.go:486:			ms.StoreMessageInfo(mi)
internal/shimdiag/shimdiag.pb.go:524:	ms.StoreMessageInfo(mi)
internal/shimdiag/shimdiag.pb.go:537:		if ms.LoadMessageInfo() == nil {
internal/shimdiag/shimdiag.pb.go:538:			ms.StoreMessageInfo(mi)
internal/shimdiag/shimdiag.pb.go:575:	ms.StoreMessageInfo(mi)
internal/shimdiag/shimdiag.pb.go:588:		if ms.LoadMessageInfo() == nil {
internal/shimdiag/shimdiag.pb.go:589:			ms.StoreMessageInfo(mi)
internal/tools/extendedtask/extendedtask.go:42:		resp, err := svc.ComputeProcessorInfo(context.Background(), &extendedtask.ComputeProcessorInfoRequest{ID: containerID})
internal/tools/networkagent/main.go:144:	log.G(ctx).WithField("endpt", endpt).Info("ConfigureContainerNetworking created endpoint")
internal/tools/networkagent/main.go:225:	log.G(ctx).WithField("endpt", endpt).Info("ConfigureContainerNetworking created endpoint")
internal/tools/networkagent/main.go:264:			log.G(ctx).Warn("failed to find endpoint to delete")
internal/tools/networkagent/main.go:268:			log.G(ctx).WithField("name", endpoint.ID).Warn("failed to get endpoint settings")
internal/tools/networkagent/main.go:279:				log.G(ctx).WithField("name", endpoint.ID).Warn("invalid endpoint settings type")
internal/tools/networkagent/main.go:286:				log.G(ctx).WithField("name", endpointName).Warn("failed to delete endpoint")
internal/tools/networkagent/main.go:297:				log.G(ctx).WithField("name", networkName).Warn("failed to delete network")
internal/tools/networkagent/main.go:308:	log.G(ctx).WithField("req", req).Info("ConfigureContainerNetworking request")
internal/tools/networkagent/main.go:342:	log.G(ctx).WithField("endpts", resp.Endpoints).Info("ConfigureNetworking addrequest")
internal/tools/networkagent/main.go:346:			log.G(ctx).Warn("failed to find endpoint")
internal/tools/networkagent/main.go:350:			log.G(ctx).WithField("name", endpoint.ID).Warn("failed to get endpoint settings")
internal/tools/networkagent/main.go:366:				log.G(ctx).WithField("name", endpoint.ID).Warn("invalid endpoint settings type")
internal/tools/networkagent/main.go:399:			log.G(ctx).Warn("failed to find endpoint to delete")
internal/tools/networkagent/main.go:403:			log.G(ctx).WithField("name", endpoint.ID).Warn("failed to get endpoint settings")
internal/tools/networkagent/main.go:415:				log.G(ctx).WithField("name", endpoint.ID).Warn("invalid endpoint settings type")
internal/tools/networkagent/main.go:420:				log.G(ctx).WithField("name", endpointName).Warn("endpoint was not assigned a NIC ID previously")
internal/tools/networkagent/main.go:430:				log.G(ctx).WithField("name", endpointName).Warn("failed to delete endpoint nic")
internal/tools/networkagent/main.go:439:	log.G(ctx).WithField("req", req).Info("ConfigureNetworking request")
internal/tools/networkagent/main.go:468:		log.G(ctx).WithError(err).Fatalf("failed to read network agent's config file at %s", *configPath)
internal/tools/networkagent/main.go:473:	}).Info("network agent configuration")
internal/tools/networkagent/main.go:488:		log.G(ctx).WithError(err).Fatalf("failed to connect to ncproxy at %s", conf.GRPCAddr)
internal/tools/networkagent/main.go:492:	log.G(ctx).WithField("addr", conf.GRPCAddr).Info("connected to ncproxy")
internal/tools/networkagent/main.go:510:		log.G(ctx).WithError(err).Fatalf("failed to listen on %s", grpcListener.Addr().String())
internal/tools/networkagent/main.go:516:			if strings.Contains(err.Error(), "use of closed network connection") {
internal/tools/networkagent/main.go:523:	log.G(ctx).WithField("addr", conf.NodeNetSvcAddr).Info("serving network service agent")
internal/tools/networkagent/main.go:528:		log.G(ctx).Info("Received interrupt. Closing")
internal/tools/networkagent/main.go:531:			log.G(ctx).WithError(err).Fatal("grpc service failure")
internal/tools/networkagent/v0_service_wrapper.go:53:	log.G(ctx).WithField("req", req).Info("ConfigureNetworking request")
internal/tools/rootfs/main.go:73:				}).Error(ctx.App.Name + " failed")
internal/tools/rootfs/merge.go:150:	}).Info("merging layers")
internal/tools/rootfs/merge.go:181:	}).Info("merged layer tarball")
internal/tools/rootfs/merge.go:215:		}).Debug("finished processing image layers")
internal/tools/rootfs/merge.go:260:			"directory": header.FileInfo().IsDir(),
internal/tools/rootfs/merge.go:399:			entry.Warn("existing output file will be overwritten")
internal/tools/rootfs/merge.go:403:		entry.WithError(err).Warn("unable to stat")
internal/tools/rootfs/merge.go:406:	entry.Debug("using output path")
internal/tools/uvmboot/conf_wcow.go:194:			return vm.ExitError()
internal/tools/uvmboot/lcow.go:205:			return nil, unrecognizedError(c.String(kernelFileArgName), kernelFileArgName)
internal/tools/uvmboot/lcow.go:225:			return nil, unrecognizedError(c.String(rootFSTypeArgName), rootFSTypeArgName)
internal/tools/uvmboot/lcow.go:257:				return nil, unrecognizedError(c.String(outputHandlingArgName), outputHandlingArgName)
internal/tools/uvmboot/lcow.go:307:				entry.WithField(logfields.Value+"-existing", vv).Warn("overriding existing annotation")
internal/tools/uvmboot/lcow.go:349:		return vm.ExitError()
internal/tools/uvmboot/lcow.go:364:			log.G(ctx).WithError(err).Warn("could not create console from stdin")
internal/tools/uvmboot/main.go:160:					logrus.WithField("uvm-id", id).WithError(err).Error("failed to run UVM")
internal/tools/uvmboot/main.go:179:func unrecognizedError(name, value string) error {
internal/tools/uvmboot/mounts.go:38:		}).Info("Mounted SCSI disk")
internal/tools/uvmboot/mounts.go:66:		}).Debug("Shared path")
internal/tools/uvmboot/mounts.go:102:			entry.WithError(err).Warnf("invald %s flag value", name)
internal/tools/uvmboot/wcow.go:132:			return vm.ExitError()
internal/uvm/computeagent.go:70:	}).Info("AssignPCI request")
internal/uvm/computeagent.go:73:		return nil, status.Error(codes.InvalidArgument, "received empty field in request")
internal/uvm/computeagent.go:87:	}).Info("RemovePCI request")
internal/uvm/computeagent.go:90:		return nil, status.Error(codes.InvalidArgument, "received empty field in request")
internal/uvm/computeagent.go:104:	}).Info("AddNIC request")
internal/uvm/computeagent.go:107:		return nil, status.Error(codes.InvalidArgument, "received empty field in request")
internal/uvm/computeagent.go:158:		return nil, status.Error(codes.InvalidArgument, "invalid request endpoint type")
internal/uvm/computeagent.go:169:	}).Info("ModifyNIC request")
internal/uvm/computeagent.go:172:		return nil, status.Error(codes.InvalidArgument, "received empty field in request")
internal/uvm/computeagent.go:208:		return nil, status.Error(codes.InvalidArgument, "invalid request endpoint type")
internal/uvm/computeagent.go:220:	}).Info("DeleteNIC request")
internal/uvm/computeagent.go:223:		return nil, status.Error(codes.InvalidArgument, "received empty field in request")
internal/uvm/computeagent.go:248:		return nil, status.Error(codes.InvalidArgument, "invalid request endpoint type")
internal/uvm/computeagent.go:266:	log.G(ctx).WithField("address", l.Addr().String()).Info("serving compute agent")
internal/uvm/computeagent.go:270:			log.G(ctx).WithError(err).Fatal("compute agent: serve failure")
internal/uvm/computeagent.go:278:	if err == nil || strings.Contains(err.Error(), "use of closed network connection") {
internal/uvm/create.go:285:	}).Debug("created utility VM")
internal/uvm/create.go:338:		e.Debug("removing VMGS file")
internal/uvm/create.go:340:			e.WithError(err).Error("failed to remove VMGS file")
internal/uvm/create.go:394:func (uvm *UtilityVM) ExitError() error {
internal/uvm/create.go:395:	return uvm.hcsSystem.ExitError()
internal/uvm/create_lcow.go:179:		}).Debug("updated LCOW root filesystem to " + vmutils.VhdFile)
internal/uvm/create_lcow.go:193:			}).Debug("updated LCOW kernel file to " + vmutils.UncompressedKernelFile)
internal/uvm/create_lcow.go:200:	processorTopology, err := processorinfo.HostProcessorInfo(ctx)
internal/uvm/create_lcow.go:420:		logrus.Debug("makeLCOWVMGSDoc configuring scsi devices")
internal/uvm/create_lcow.go:428:			logrus.Debug("makeLCOWVMGSDoc DmVerityMode true")
internal/uvm/create_lcow.go:630:		log.G(ctx).WithField("resource-partition-id", opts.ResourcePartitionID.String()).Debug("setting resource partition ID")
internal/uvm/create_lcow.go:740:							log.G(ctx).WithError(err).Debug("failed to release memory region")
internal/uvm/create_lcow.go:751:				dev := newDefaultVPMemInfo(opts.RootFSFile, "/")
internal/uvm/create_lcow.go:843:			log.G(ctx).Warn("ignoring `WritableOverlayDirs` option since rootfs is already writable")
internal/uvm/create_lcow.go:897:		log.G(ctx).WithField("options", log.Format(ctx, opts)).Debug("uvm::CreateLCOW options")
internal/uvm/create_lcow.go:935:		return nil, errors.Wrap(err, errBadUVMOpts.Error())
internal/uvm/create_lcow.go:986:		log.G(ctx).WithField("vmID", uvm.runtimeID).Debug("Using external GCS bridge")
internal/uvm/create_test.go:18:	if err == nil || err.Error() != `kernel: 'c:\does\not\exist\I\hope\kernel' not found` {
internal/uvm/create_wcow.go:130:	}).Debug("Using external GCS bridge")
internal/uvm/create_wcow.go:145:	processorTopology, err := processorinfo.HostProcessorInfo(ctx)
internal/uvm/create_wcow.go:292:		log.G(ctx).WithField("resource-partition-id", opts.ResourcePartitionID.String()).Debug("setting resource partition ID")
internal/uvm/create_wcow.go:564:	log.G(ctx).WithField("options", log.Format(ctx, opts)).Debug("uvm::CreateWCOW options")
internal/uvm/create_wcow.go:593:		return nil, errors.Wrap(err, errBadUVMOpts.Error())
internal/uvm/log_wcow.go:33:		log.G(ctx).WithField("os", uvm.operatingSystem).Error("Log forwarding not supported for this OS")
internal/uvm/modify.go:32:					log.G(ctx).WithError(rerr).Error("failed to roll back resource add")
internal/uvm/modify.go:45:			log.G(ctx).WithError(err).Error("failed to remove host resources after successful guest request")
internal/uvm/network.go:90:			log.G(ctx).Warn(removeErr)
internal/uvm/network.go:101:	l.Debug(op + " - Begin")
internal/uvm/network.go:103:		l.Debug(op + " - End")
internal/uvm/network.go:316:			}).Warn("removing endpoint from namespace: does not exist")
internal/uvm/scsi/manager_test.go:137:		t.Error("guest path for m1 should not be empty")
internal/uvm/security_policy.go:51:func WithUVMReferenceInfo(referenceRoot string, referenceName string) ConfidentialUVMOpt {
internal/uvm/security_policy.go:60:				log.G(ctx).WithField("filePath", fullFilePath).Debug("UVM reference info file not found")
internal/uvm/start.go:92:				e.WithError(err).Error("failed to connect to entropy socket")
internal/uvm/start.go:98:				e.WithError(err).Error("failed to write entropy")
internal/uvm/start.go:121:						e.WithError(err).Error("failed to connect to log socket")
internal/uvm/start.go:127:						e.Info("uvm output handler starting")
internal/uvm/start.go:130:					e.Info("uvm output handler finished")
internal/uvm/start.go:143:					e.WithError(err).Error("failed to connect to log socket")
internal/uvm/start.go:151:					e.Debug("uvm output handler finished")
internal/uvm/start.go:178:			err = uvm.hcsSystem.ExitError()
internal/uvm/start.go:282:			WithUVMReferenceInfo(referenceInfoFileRoot, referenceInfoFilePath),
internal/uvm/start.go:293:			e.WithError(err).Error("failed to set log sources")
internal/uvm/start.go:296:			e.WithError(err).Error("failed to start log forwarding")
internal/uvm/stats.go:64:		}).Debug("checking vmmem process identity")
internal/uvm/stats.go:78:	log.G(ctx).WithField("vmID", vmIDStr).Debug("looking up vmmem")
internal/uvm/stats.go:91:			log.G(ctx).WithField("pid", pid).Debug("failed to check process")
internal/uvm/stats.go:95:			log.G(ctx).WithField("pid", pid).Debug("found vmmem match")
internal/uvm/stats.go:139:		memCounters, err := process.GetProcessMemoryInfo(vmmemProc)
internal/uvm/vpmem.go:50:func newDefaultVPMemInfo(hostPath, uvmPath string) *vPMemInfoDefault {
internal/uvm/vpmem.go:67:			}).Debug("allocated VPMem location")
internal/uvm/vpmem.go:85:			}).Debug("found VPMem location")
internal/uvm/vpmem.go:140:	uvm.vpmemDevicesDefault[deviceNumber] = newDefaultVPMemInfo(hostPath, uvmPath)
internal/uvm/vpmem.go:178:	}).Debug("removed VPMEM location")
internal/uvm/vpmem_mapped.go:143:	}).Debug("mapped new device")
internal/uvm/vpmem_mapped.go:184:				}).Debug("found mapped VHD")
internal/uvm/vpmem_mapped.go:217:		}).Debug("found offset for mapped VHD on an existing VPMem device")
internal/uvm/vpmem_mapped.go:250:				log.G(ctx).WithError(err).Debugf("failed to reclaim pmem region: %s", err)
internal/uvm/vpmem_mapped.go:265:				log.G(ctx).WithError(err).Debugf("failed to rollback modification")
internal/uvm/vpmem_mapped.go:312:		log.G(ctx).WithError(err).Debugf("failed unmapping VHD layer %s", hostPath)
internal/uvm/vsmb.go:186:		log.G(ctx).WithField("path", hostPath).Info("Forcing NoDirectmap for VSMB mount")
internal/uvm/vsmb.go:221:		}).Info("Modifying VSMB share")
internal/uvm/vsmb.go:304:		}).Debug("skipping remove of directmapped vSMB share")
internal/uvm/wait.go:22:	logrus.WithField(logfields.UVMID, uvm.id).Debug("uvm exited, waiting for output processing to complete")
internal/uvmfolder/locate.go:39:	}).Debug("hcsshim::LocateUVMFolder: found")
internal/verity/verity.go:37:	dmvsb, err := dmverity.ReadDMVerityInfo(layerPath, ext4SizeInBytes)
internal/verity/verity.go:48:	}).Debug("dm-verity information")
internal/vhdx/info.go:146:func GetScratchVhdPartitionInfo(ctx context.Context, vhdxPath string) (_ ScratchVhdxPartitionInfo, err error) {
internal/vhdx/info.go:172:			}).Warn("failed to close vhd handle")
internal/vhdx/info.go:186:			}).Warn("failed to detach vhd")
internal/vhdx/info.go:229:	}).Debug("Scratch VHD partition info")
internal/vm/vmmanager/lifetime.go:60:	ExitError() error
internal/vm/vmmanager/lifetime.go:152:func (uvm *UtilityVM) ExitError() error {
internal/vm/vmmanager/lifetime.go:153:	return uvm.cs.ExitError()
internal/vm/vmmanager/utils.go:50:	return nil, uvm.ExitError()
internal/vm/vmmanager/uvm.go:62:	}).Debug("created utility VM")
internal/vm/vmutils/etw/provider_map_test.go:34:	originalDefaults := cloneLogSourcesInfo(defaultLogSourcesInfo)
internal/vm/vmutils/etw/provider_map_test.go:36:		defaultLogSourcesInfo = cloneLogSourcesInfo(originalDefaults)
internal/vm/vmutils/etw/provider_map_test.go:108:			defaultLogSourcesInfo = cloneLogSourcesInfo(originalDefaults)
internal/vm/vmutils/etw/provider_map_test.go:163:		result = cloneLogSourcesInfo(defaults)
internal/vm/vmutils/etw/provider_map_test.go:167:		userCopy := cloneLogSourcesInfo(user)
internal/vm/vmutils/etw/provider_map_test.go:208:func cloneLogSourcesInfo(in LogSourcesInfo) LogSourcesInfo {
internal/vm/vmutils/etw/provider_map_test.go:250:	originalDefaults := cloneLogSourcesInfo(defaultLogSourcesInfo)
internal/vm/vmutils/etw/provider_map_test.go:252:		defaultLogSourcesInfo = cloneLogSourcesInfo(originalDefaults)
internal/vm/vmutils/etw/provider_map_test.go:372:			defaultLogSourcesInfo = cloneLogSourcesInfo(originalDefaults)
internal/vm/vmutils/etw/provider_map_test.go:378:			if !strings.Contains(err.Error(), tt.errContains) {
internal/vm/vmutils/gcs_logs.go:71:					}).Error("gcs log read")
internal/vm/vmutils/gcs_logs.go:82:					}).Error("gcs terminated")
internal/vm/vmutils/gcs_logs_test.go:117:					t.Error("expected error but got none")
internal/vm/vmutils/gcs_logs_test.go:326:		t.Error("expected to find 'last message' entry")
internal/vm/vmutils/gcs_logs_test.go:336:				t.Error("stderr field not found or not a string")
internal/vm/vmutils/gcs_logs_test.go:344:		t.Error("expected to find 'gcs terminated' entry")
internal/vm/vmutils/gcs_logs_test.go:370:		t.Error("vm.time field not found")
internal/vm/vmutils/gcs_logs_test.go:388:		t.Error("ParseGCSLogrus should return a non-nil handler")
internal/vm/vmutils/normalize.go:32:		}).Warn("Changing user requested MemorySizeInMB to align to 2MB")
internal/vm/vmutils/normalize.go:53:		}).Warn("Changing user requested CPUCount to current number of processors")
internal/vm/vmutils/numa.go:69:			}).Warn("potentially incomplete implicit vNUMA topology")
internal/vm/vmutils/numa.go:76:			}).Warn("potentially incomplete explicit vNUMA topology")
internal/vm/vmutils/numa.go:110:			}).Debug("created implicit NUMA topology")
internal/vm/vmutils/numa.go:139:		entry.WithField("numa", log.Format(ctx, numa)).Debug("created explicit NUMA topology")
internal/vm/vmutils/numa_test.go:318:				} else if tc.errMsg != "" && !containsSubstring(err.Error(), tc.errMsg) {
internal/vm/vmutils/numa_test.go:319:					t.Errorf("validate() error = %q, expected to contain %q", err.Error(), tc.errMsg)
internal/vm/vmutils/numa_test.go:547:				} else if tc.errMsg != "" && !containsSubstring(err.Error(), tc.errMsg) {
internal/vm/vmutils/numa_test.go:548:					t.Errorf("ValidateNumaForVM() error = %q, expected to contain %q", err.Error(), tc.errMsg)
internal/vm/vmutils/numa_test.go:824:				} else if tc.errMsg != "" && !containsSubstring(err.Error(), tc.errMsg) {
internal/vm/vmutils/numa_test.go:825:					t.Errorf("PrepareVNumaTopology() error = %q, expected to contain %q", err.Error(), tc.errMsg)
internal/vm/vmutils/numa_test.go:835:				t.Error("PrepareVNumaTopology() expected Numa result, got nil")
internal/vm/vmutils/numa_test.go:842:				t.Error("PrepareVNumaTopology() expected NumaProcessors result, got nil")
internal/vm/vmutils/numa_test.go:874:	if !containsSubstring(err.Error(), "vNUMA topology is not supported") {
internal/vm/vmutils/numa_test.go:875:		t.Errorf("PrepareVNumaTopology() error = %q, expected to contain 'vNUMA topology is not supported'", err.Error())
internal/vm/vmutils/utils.go:21:func ParseUVMReferenceInfo(ctx context.Context, referenceRoot, referenceName string) (string, error) {
internal/vm/vmutils/utils.go:30:			log.G(ctx).WithField("filePath", fullFilePath).Debug("UVM reference info file not found")
internal/vm/vmutils/utils.go:59:		entry.WithField("options", log.Format(ctx, shimOpts)).Debug("parsed runtime options")
internal/vm/vmutils/vmmem.go:35:			log.G(ctx).WithError(err).Error("failed to create process snapshot")
internal/vm/vmutils/vmmem.go:47:				log.G(ctx).WithError(err).Debug("finished iterating process entries")
internal/vm/vmutils/vmmem.go:62:	log.G(ctx).WithField("vmID", vmIDStr).Debug("looking up vmmem via LookupAccount")
internal/vm/vmutils/vmmem.go:101:			log.G(ctx).WithField("pid", pe32.ProcessID).Debug("found vmmem match")
internal/vm/vmutils/vmmem_test.go:268:				} else if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
internal/vmcompute/vmcompute.go:43://sys hcsGetProcessInfo(process HcsProcess, processInformation *HcsProcessInformation, result **uint16) (hr error) = vmcompute.HcsGetProcessInfo?
internal/vmcompute/vmcompute.go:503:func HcsGetProcessInfo(ctx gcontext.Context, process HcsProcess) (processInformation HcsProcessInformation, result string, hr error) {
internal/vmcompute/vmcompute.go:515:		err := hcsGetProcessInfo(process, &processInformation, &resultp)
internal/vmcompute/zsyscall_windows.go:201:func hcsGetProcessInfo(process HcsProcess, processInformation *HcsProcessInformation, result **uint16) (hr error) {
internal/wclayer/baselayerreader.go:175:	fileInfo, err = winio.GetFileBasicInfo(f)
internal/wclayer/baselayerreader.go:188:func (r *baseLayerReader) LinkInfo() (uint32, *winio.FileIDInfo, error) {
internal/wclayer/baselayerreader.go:189:	fileStandardInfo, err := winio.GetFileStandardInfo(r.currentFile)
internal/wclayer/baselayerwriter.go:48:		err = winio.SetFileBasicInfo(f, &di.fileInfo)
internal/wclayer/baselayerwriter.go:108:	err = winio.SetFileBasicInfo(f, fileInfo)
internal/wclayer/cim/block_cim_writer.go:64:				log.G(ctx).WithError(err).Warnf("failed to close cim after error: %s", cErr)
internal/wclayer/cim/block_cim_writer.go:112:		log.G(cw.ctx).Warn("UtilityVM files in non base layers is not supported for block CIMs")
internal/wclayer/cim/forked_cim_writer.go:45:				log.G(ctx).WithError(err).Warnf("failed to close cim after error: %s", cErr)
internal/wclayer/cim/forked_cim_writer.go:49:				log.G(ctx).WithError(err).Warnf("failed to cleanup cim after error: %s", cErr)
internal/wclayer/cim/mount.go:72:	}).Debug("mounting block layer CIM")
internal/wclayer/cim/mount.go:131:	}).Debug("cleanup container CIM mounts")
internal/wclayer/exportlayer.go:49:	LinkInfo() (uint32, *winio.FileIDInfo, error)
internal/wclayer/getlayermountpath.go:29:	log.G(ctx).Debug("Calling proc (1)")
internal/wclayer/getlayermountpath.go:43:	log.G(ctx).Debug("Calling proc (2)")
internal/wclayer/layerutils.go:89:			logrus.WithError(err).Debug("Failed to convert name to guid")
internal/wclayer/layerutils.go:95:			logrus.WithError(err).Debug("Failed conversion of parentLayerPath to pointer")
internal/wclayer/legacy.go:248:	fileInfo, err = winio.GetFileBasicInfo(f)
internal/wclayer/legacy.go:265:			fileInfo, err = winio.GetFileBasicInfo(g)
internal/wclayer/legacy.go:307:func (r *legacyLayerReader) LinkInfo() (uint32, *winio.FileIDInfo, error) {
internal/wclayer/legacy.go:308:	fileStandardInfo, err := winio.GetFileStandardInfo(r.currentFile)
internal/wclayer/legacy.go:507:	fileInfo, err = winio.GetFileBasicInfo(src)
internal/wclayer/legacy.go:528:	err = winio.SetFileBasicInfo(dest, fileInfo)
internal/wclayer/legacy.go:669:		err = winio.SetFileBasicInfo(f, fileInfo)
internal/wclayer/legacy.go:707:	err = winio.SetFileBasicInfo(f, &strippedFi)
internal/winapi/cimfs.go:17:		logrus.Info("using cimwriter.dll for CIM write operations")
internal/winapi/cimfs.go:19:		logrus.Info("using cimfs.dll for CIM write operations")
internal/winapi/cimfs.go:21:		logrus.Warn("no CIM DLL available for write operations")
internal/winapi/cimfs/cimfs.go:54:		logrus.WithField("path", windows.UTF16ToString(buf[:n])).Info("loaded cimfs.dll")
internal/winapi/cimwriter/cimwriter.go:49:			logrus.WithError(freeErr).Warn("failed to free cimwriter.dll after load failure")
internal/winapi/cimwriter/cimwriter.go:56:		logrus.WithField("path", windows.UTF16ToString(buf[:n])).Info("loaded cimwriter.dll")
internal/winapi/errors.go:7://sys RtlNtStatusToDosError(status uint32) (winerr error) = ntdll.RtlNtStatusToDosError
internal/winapi/user.go:63:// NET_API_STATUS NET_API_FUNCTION NetLocalGroupGetInfo(
internal/winapi/user.go:70://sys netLocalGroupGetInfo(serverName *uint16, groupName *uint16, level uint32, bufptr **byte) (status error) = netapi32.NetLocalGroupGetInfo
internal/winapi/user.go:74:func NetLocalGroupGetInfo(serverName, groupName string, level uint32, bufPtr **byte) (err error) {
internal/winapi/user.go:91:	return netLocalGroupGetInfo(
internal/winapi/zsyscall_windows.go:359:func netLocalGroupGetInfo(serverName *uint16, groupName *uint16, level uint32, bufptr **byte) (status error) {
internal/winapi/zsyscall_windows.go:453:func RtlNtStatusToDosError(status uint32) (winerr error) {
internal/windevice/devicequery_test.go:121:	if err != nil && !strings.Contains(err.Error(), "An instance of the service is already running") {
internal/winobjdir/object_dir.go:47:		return nil, winapi.RtlNtStatusToDosError(status)
internal/winobjdir/object_dir.go:70:			return nil, winapi.RtlNtStatusToDosError(status)
pkg/amdsevsnp/report_linux.go:211:func CheckDriverError() error {
pkg/amdsevsnp/report_windows.go:112:func CheckDriverError() error {
pkg/amdsevsnp/validate.go:14:	if err := CheckDriverError(); err != nil {
pkg/cimfs/cim_test.go:712:	rootHash, err := GetVerificationInfo(blockPath)
pkg/cimfs/cim_test.go:757:	rootHash, err := GetVerificationInfo(blockPath)
pkg/cimfs/cim_test.go:774:	} else if !strings.Contains(err.Error(), "integrity violation") {
pkg/cimfs/cim_test.go:855:			rootHash, err := GetVerificationInfo(mergedBlockPath)
pkg/cimfs/cim_writer_windows.go:365:		log.G(ctx).WithError(err).Warnf("get region files for cim %s", cimPath)
pkg/cimfs/cim_writer_windows.go:372:		log.G(ctx).WithError(err).Warnf("get objectid file for cim %s", cimPath)
pkg/cimfs/cim_writer_windows.go:382:	}).Debug("destroy cim")
pkg/cimfs/cim_writer_windows.go:386:			log.G(ctx).WithError(err).Warnf("remove file %s", regFilePath)
pkg/cimfs/cim_writer_windows.go:395:			log.G(ctx).WithError(err).Warnf("remove file %s", objFilePath)
pkg/cimfs/cim_writer_windows.go:403:		log.G(ctx).WithError(err).Warnf("remove file %s", cimPath)
pkg/cimfs/cim_writer_windows.go:526:func GetVerificationInfo(blockPath string) ([]byte, error) {
pkg/cimfs/cimfs.go:19:		logrus.WithError(err).Warn("get build revision")
pkg/cimfs/common.go:33:func (e *OpError) Error() string {
pkg/cimfs/common.go:35:	s += ": " + e.Err.Error()
pkg/cimfs/common.go:47:func (e *PathError) Error() string {
pkg/cimfs/common.go:50:	s += ": " + e.Err.Error()
pkg/cimfs/common.go:62:func (e *LinkError) Error() string {
pkg/cimfs/common.go:63:	return "cim " + e.Op + " " + e.Old + " " + e.New + ": " + e.Err.Error()
pkg/cimfs/common.go:108:			log.G(ctx).WithError(err).Warnf("stat for object file %s", path)
pkg/cimfs/common.go:134:			log.G(ctx).WithError(err).Warnf("stat for region file %s", path)
pkg/cimfs/mount_cim.go:26:func (e *MountError) Error() string {
pkg/cimfs/mount_cim.go:31:	s += " " + e.VolumeGUID.String() + ": " + e.Err.Error()
pkg/go-runhcs/runhcs.go:143:func (r *Runhcs) runOrError(cmd *exec.Cmd) error {
pkg/go-runhcs/runhcs_create-scratch.go:61:	return r.runOrError(r.command(ctx, args...))
pkg/go-runhcs/runhcs_delete.go:34:	return r.runOrError(r.command(ctx, append(args, id)...))
pkg/go-runhcs/runhcs_kill.go:12:	return r.runOrError(r.command(ctx, "kill", id, signal))
pkg/go-runhcs/runhcs_pause.go:11:	return r.runOrError(r.command(ctx, "pause", id))
pkg/go-runhcs/runhcs_resize-tty.go:34:	return r.runOrError(r.command(ctx, append(args, id, strconv.FormatUint(uint64(width), 10), strconv.FormatUint(uint64(height), 10))...))
pkg/go-runhcs/runhcs_resume.go:11:	return r.runOrError(r.command(ctx, "resume", id))
pkg/go-runhcs/runhcs_start.go:11:	return r.runOrError(r.command(ctx, "start", id))
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:139:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:152:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:153:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:200:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:213:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:214:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:245:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:258:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:259:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:314:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:327:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:328:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:357:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:370:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:371:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:418:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:431:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:432:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:467:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:480:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:481:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:554:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:567:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:568:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:601:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:614:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:615:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:652:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:665:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:666:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:719:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:732:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:733:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:796:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:809:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:810:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:899:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:912:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:913:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:948:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:961:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:962:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:1001:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:1014:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:1015:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:1040:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:1053:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:1054:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:1085:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:1098:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:1099:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:1124:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:1137:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:1138:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:1169:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:1182:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:1183:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:1208:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:1221:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:1222:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:1263:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:1276:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:1277:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:1342:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:1355:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:1356:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:1391:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:1404:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:1405:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:1444:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:1457:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:1458:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:1483:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:1496:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:1497:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:1528:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:1541:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:1542:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:1567:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:1580:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v0/networkconfigproxy.pb.go:1581:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:130:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:143:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:144:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:194:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:207:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:208:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:234:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:247:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:248:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:298:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:311:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:312:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:337:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:350:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:351:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:394:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:407:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:408:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:431:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:444:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:445:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:479:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:492:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:493:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:557:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:570:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:571:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:608:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:621:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:622:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:701:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:714:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:715:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:745:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:758:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:759:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:791:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:804:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:805:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:851:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:864:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:865:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:909:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:922:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:923:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:957:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:970:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:971:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1037:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1050:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1051:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1103:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1116:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1117:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1204:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1217:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1218:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1266:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1279:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1280:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1386:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1399:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1400:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1437:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1450:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1451:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1483:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1496:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1497:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1540:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1553:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1554:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1577:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1590:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1591:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1620:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1633:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1634:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1657:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1670:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1671:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1700:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1713:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1714:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1737:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1750:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1751:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1783:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1796:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1797:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1841:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1854:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1855:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1887:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1900:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1901:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1946:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1959:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1960:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:1996:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:2009:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:2010:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:2033:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:2046:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:2047:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:2076:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:2089:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:2090:			ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:2113:	ms.StoreMessageInfo(mi)
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:2126:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/ncproxygrpc/v1/networkconfigproxy.pb.go:2127:			ms.StoreMessageInfo(mi)
pkg/ncproxy/nodenetsvc/v0/nodenetsvc.pb.go:88:	ms.StoreMessageInfo(mi)
pkg/ncproxy/nodenetsvc/v0/nodenetsvc.pb.go:101:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/nodenetsvc/v0/nodenetsvc.pb.go:102:			ms.StoreMessageInfo(mi)
pkg/ncproxy/nodenetsvc/v0/nodenetsvc.pb.go:141:	ms.StoreMessageInfo(mi)
pkg/ncproxy/nodenetsvc/v0/nodenetsvc.pb.go:154:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/nodenetsvc/v0/nodenetsvc.pb.go:155:			ms.StoreMessageInfo(mi)
pkg/ncproxy/nodenetsvc/v0/nodenetsvc.pb.go:180:	ms.StoreMessageInfo(mi)
pkg/ncproxy/nodenetsvc/v0/nodenetsvc.pb.go:193:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/nodenetsvc/v0/nodenetsvc.pb.go:194:			ms.StoreMessageInfo(mi)
pkg/ncproxy/nodenetsvc/v0/nodenetsvc.pb.go:227:	ms.StoreMessageInfo(mi)
pkg/ncproxy/nodenetsvc/v0/nodenetsvc.pb.go:240:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/nodenetsvc/v0/nodenetsvc.pb.go:241:			ms.StoreMessageInfo(mi)
pkg/ncproxy/nodenetsvc/v0/nodenetsvc.pb.go:278:	ms.StoreMessageInfo(mi)
pkg/ncproxy/nodenetsvc/v0/nodenetsvc.pb.go:291:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/nodenetsvc/v0/nodenetsvc.pb.go:292:			ms.StoreMessageInfo(mi)
pkg/ncproxy/nodenetsvc/v0/nodenetsvc.pb.go:341:	ms.StoreMessageInfo(mi)
pkg/ncproxy/nodenetsvc/v0/nodenetsvc.pb.go:354:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/nodenetsvc/v0/nodenetsvc.pb.go:355:			ms.StoreMessageInfo(mi)
pkg/ncproxy/nodenetsvc/v0/nodenetsvc.pb.go:394:	ms.StoreMessageInfo(mi)
pkg/ncproxy/nodenetsvc/v0/nodenetsvc.pb.go:407:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/nodenetsvc/v0/nodenetsvc.pb.go:408:			ms.StoreMessageInfo(mi)
pkg/ncproxy/nodenetsvc/v0/nodenetsvc.pb.go:471:	ms.StoreMessageInfo(mi)
pkg/ncproxy/nodenetsvc/v0/nodenetsvc.pb.go:484:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/nodenetsvc/v0/nodenetsvc.pb.go:485:			ms.StoreMessageInfo(mi)
pkg/ncproxy/nodenetsvc/v0/nodenetsvc.pb.go:542:	ms.StoreMessageInfo(mi)
pkg/ncproxy/nodenetsvc/v0/nodenetsvc.pb.go:555:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/nodenetsvc/v0/nodenetsvc.pb.go:556:			ms.StoreMessageInfo(mi)
pkg/ncproxy/nodenetsvc/v0/nodenetsvc.pb.go:589:	ms.StoreMessageInfo(mi)
pkg/ncproxy/nodenetsvc/v0/nodenetsvc.pb.go:602:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/nodenetsvc/v0/nodenetsvc.pb.go:603:			ms.StoreMessageInfo(mi)
pkg/ncproxy/nodenetsvc/v1/nodenetsvc.pb.go:82:	ms.StoreMessageInfo(mi)
pkg/ncproxy/nodenetsvc/v1/nodenetsvc.pb.go:95:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/nodenetsvc/v1/nodenetsvc.pb.go:96:			ms.StoreMessageInfo(mi)
pkg/ncproxy/nodenetsvc/v1/nodenetsvc.pb.go:132:	ms.StoreMessageInfo(mi)
pkg/ncproxy/nodenetsvc/v1/nodenetsvc.pb.go:145:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/nodenetsvc/v1/nodenetsvc.pb.go:146:			ms.StoreMessageInfo(mi)
pkg/ncproxy/nodenetsvc/v1/nodenetsvc.pb.go:169:	ms.StoreMessageInfo(mi)
pkg/ncproxy/nodenetsvc/v1/nodenetsvc.pb.go:182:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/nodenetsvc/v1/nodenetsvc.pb.go:183:			ms.StoreMessageInfo(mi)
pkg/ncproxy/nodenetsvc/v1/nodenetsvc.pb.go:213:	ms.StoreMessageInfo(mi)
pkg/ncproxy/nodenetsvc/v1/nodenetsvc.pb.go:226:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/nodenetsvc/v1/nodenetsvc.pb.go:227:			ms.StoreMessageInfo(mi)
pkg/ncproxy/nodenetsvc/v1/nodenetsvc.pb.go:259:	ms.StoreMessageInfo(mi)
pkg/ncproxy/nodenetsvc/v1/nodenetsvc.pb.go:272:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/nodenetsvc/v1/nodenetsvc.pb.go:273:			ms.StoreMessageInfo(mi)
pkg/ncproxy/nodenetsvc/v1/nodenetsvc.pb.go:317:	ms.StoreMessageInfo(mi)
pkg/ncproxy/nodenetsvc/v1/nodenetsvc.pb.go:330:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/nodenetsvc/v1/nodenetsvc.pb.go:331:			ms.StoreMessageInfo(mi)
pkg/ncproxy/nodenetsvc/v1/nodenetsvc.pb.go:364:	ms.StoreMessageInfo(mi)
pkg/ncproxy/nodenetsvc/v1/nodenetsvc.pb.go:377:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/nodenetsvc/v1/nodenetsvc.pb.go:378:			ms.StoreMessageInfo(mi)
pkg/ncproxy/nodenetsvc/v1/nodenetsvc.pb.go:432:	ms.StoreMessageInfo(mi)
pkg/ncproxy/nodenetsvc/v1/nodenetsvc.pb.go:445:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/nodenetsvc/v1/nodenetsvc.pb.go:446:			ms.StoreMessageInfo(mi)
pkg/ncproxy/nodenetsvc/v1/nodenetsvc.pb.go:497:	ms.StoreMessageInfo(mi)
pkg/ncproxy/nodenetsvc/v1/nodenetsvc.pb.go:510:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/nodenetsvc/v1/nodenetsvc.pb.go:511:			ms.StoreMessageInfo(mi)
pkg/ncproxy/nodenetsvc/v1/nodenetsvc.pb.go:541:	ms.StoreMessageInfo(mi)
pkg/ncproxy/nodenetsvc/v1/nodenetsvc.pb.go:554:		if ms.LoadMessageInfo() == nil {
pkg/ncproxy/nodenetsvc/v1/nodenetsvc.pb.go:555:			ms.StoreMessageInfo(mi)
pkg/ociwclayer/cim/import.go:43:	}).Debug("Importing cim layer from tar")
pkg/ociwclayer/cim/import.go:104:	digest, err := cimfs.GetVerificationInfo(blockPath)
pkg/ociwclayer/cim/import.go:129:	log.G(ctx).WithField("layer", layer).Debug("Importing block CIM layer from tar")
pkg/ociwclayer/cim/import.go:143:	log.G(ctx).WithField("config", *config).Debug("layer import config")
pkg/ociwclayer/cim/import.go:284:				log.G(ctx).WithError(flushErr).Warn("flush buffer during layer write failed")
pkg/ociwclayer/cim/import.go:306:	}).Debug("Merging block CIM layers")
pkg/ociwclayer/cim/import.go:335:				log.G(ctx).WithError(retErr).Warnf("error in cleanup on failure: %s", rmErr)
pkg/ociwclayer/export.go:82:			numberOfLinks, fileIDInfo, err := r.LinkInfo()
pkg/octtrpc/interceptor.go:54:		s, ok := status.FromError(err)
pkg/octtrpc/interceptor.go:58:			span.SetStatus(trace.Status{Code: int32(codes.Internal), Message: err.Error()})
pkg/octtrpc/interceptor_test.go:44:			invokeErr:      status.Error(codes.AlreadyExists, "already exists"),
pkg/octtrpc/interceptor_test.go:91:				t.Error("expected span metadata in the request")
pkg/octtrpc/interceptor_test.go:132:			methodErr:      status.Error(codes.AlreadyExists, "already exists"),
pkg/octtrpc/interceptor_test.go:179:					t.Error("expected span to have remote parent")
pkg/securitypolicy/rego_utils_test.go:2049:	policyDecision, err := ExtractPolicyDecision(err.Error())
pkg/securitypolicy/rego_utils_test.go:2071:	policyDecision, err := ExtractPolicyDecision(err.Error())
pkg/securitypolicy/regopolicy_linux_test.go:79:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:157:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:247:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:285:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:323:			t.Error("Valid device mount failed. It shouldn't have.")
pkg/securitypolicy/regopolicy_linux_test.go:330:			t.Error("Duplicate device mount target was allowed. It shouldn't have been.")
pkg/securitypolicy/regopolicy_linux_test.go:673:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:703:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:849:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:970:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:999:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:1029:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:1066:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:1100:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:1124:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:1206:					t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:1228:					t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:1270:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:1317:		t.Error("environment variables were not dropped correctly.")
pkg/securitypolicy/regopolicy_linux_test.go:1335:		t.Error("expected container creation not to be allowed.")
pkg/securitypolicy/regopolicy_linux_test.go:1339:		t.Error("envList should be nil")
pkg/securitypolicy/regopolicy_linux_test.go:1347:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:1369:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:1394:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:1401:				t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:1423:				t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:1441:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:1461:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:1467:			t.Error("Unable to start valid container.")
pkg/securitypolicy/regopolicy_linux_test.go:1472:			t.Error("Able to start a container with already used id.")
pkg/securitypolicy/regopolicy_linux_test.go:1488:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:1502:			t.Error("Unexpected success with incorrect capabilities")
pkg/securitypolicy/regopolicy_linux_test.go:1518:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:1529:				t.Error("Unexpected success with bounding as a subset of allowed capabilities")
pkg/securitypolicy/regopolicy_linux_test.go:1541:				t.Error("Unexpected success with effective as a subset of allowed capabilities")
pkg/securitypolicy/regopolicy_linux_test.go:1553:				t.Error("Unexpected success with inheritable as a subset of allowed capabilities")
pkg/securitypolicy/regopolicy_linux_test.go:1565:				t.Error("Unexpected success with permitted as a subset of allowed capabilities")
pkg/securitypolicy/regopolicy_linux_test.go:1577:				t.Error("Unexpected success with ambient as a subset of allowed capabilities")
pkg/securitypolicy/regopolicy_linux_test.go:1594:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:1604:			t.Error("Unexpected success with bounding as a superset of allowed capabilities")
pkg/securitypolicy/regopolicy_linux_test.go:1614:			t.Error("Unexpected success with effective as a superset of allowed capabilities")
pkg/securitypolicy/regopolicy_linux_test.go:1624:			t.Error("Unexpected success with inheritable as a superset of allowed capabilities")
pkg/securitypolicy/regopolicy_linux_test.go:1634:			t.Error("Unexpected success with permitted as a superset of allowed capabilities")
pkg/securitypolicy/regopolicy_linux_test.go:1644:			t.Error("Unexpected success with ambient as a superset of allowed capabilities")
pkg/securitypolicy/regopolicy_linux_test.go:1673:		t.Error("Unexpected success with incorrect capabilities")
pkg/securitypolicy/regopolicy_linux_test.go:1769:	if err.Error() != capabilitiesNilError {
pkg/securitypolicy/regopolicy_linux_test.go:1968:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:2025:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:2050:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:2063:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:2079:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:2091:			t.Error("We added additional mounts not in policyS and it didn't result in an error")
pkg/securitypolicy/regopolicy_linux_test.go:2107:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:2135:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:2161:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:2187:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:2213:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:2226:			t.Error("We changed a mount option and it didn't result in an error")
pkg/securitypolicy/regopolicy_linux_test.go:2242:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:2255:			t.Error("We tried to mount a privileged mount when not allowed and it didn't result in an error")
pkg/securitypolicy/regopolicy_linux_test.go:2313:	if err.Error() != expected_error {
pkg/securitypolicy/regopolicy_linux_test.go:2342:		t.Error("unavailable enforcement incorrectly indicated as available")
pkg/securitypolicy/regopolicy_linux_test.go:2348:		t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:2352:		t.Error("default behavior was incorrect for unavailable enforcement point")
pkg/securitypolicy/regopolicy_linux_test.go:2378:		t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:2394:		t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:2398:		t.Error("result of allowed for an available enforcement point was not the specified default (true)")
pkg/securitypolicy/regopolicy_linux_test.go:2430:		t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:2434:		t.Error("result of allowed for an available enforcement point was not the policy value (true)")
pkg/securitypolicy/regopolicy_linux_test.go:2442:		t.Error("extra value is missing from enforcement result")
pkg/securitypolicy/regopolicy_linux_test.go:2462:		t.Error("querying an enforcement point without an api_version did not produce an error")
pkg/securitypolicy/regopolicy_linux_test.go:2465:	if err.Error() != noAPIVersionError {
pkg/securitypolicy/regopolicy_linux_test.go:2474:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:2491:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:2507:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:2522:			t.Error("Test unexpectedly passed")
pkg/securitypolicy/regopolicy_linux_test.go:2538:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:2555:			t.Error("Unexpected success when enforcing policy")
pkg/securitypolicy/regopolicy_linux_test.go:2571:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:2587:			t.Error("Unexpected success when enforcing policy")
pkg/securitypolicy/regopolicy_linux_test.go:2603:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:2620:			t.Error("Unexpected success when enforcing policy")
pkg/securitypolicy/regopolicy_linux_test.go:2636:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:2654:			t.Error("Unexpected success with bounding as a superset of allowed capabilities")
pkg/securitypolicy/regopolicy_linux_test.go:2670:			t.Error("Unexpected success with effective as a superset of allowed capabilities")
pkg/securitypolicy/regopolicy_linux_test.go:2686:			t.Error("Unexpected success with inheritable as a superset of allowed capabilities")
pkg/securitypolicy/regopolicy_linux_test.go:2702:			t.Error("Unexpected success with permitted as a superset of allowed capabilities")
pkg/securitypolicy/regopolicy_linux_test.go:2718:			t.Error("Unexpected success with ambient as a superset of allowed capabilities")
pkg/securitypolicy/regopolicy_linux_test.go:2755:	if err.Error() != capabilitiesNilError {
pkg/securitypolicy/regopolicy_linux_test.go:2794:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:2860:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:2946:			t.Error("invalid environment variables not filtered from list returned from create_container")
pkg/securitypolicy/regopolicy_linux_test.go:2953:			t.Error("invalid environment variables not filtered from list returned from exec_in_container")
pkg/securitypolicy/regopolicy_linux_test.go:2960:			t.Error("invalid environment variables not filtered from list returned from exec_external")
pkg/securitypolicy/regopolicy_linux_test.go:3003:	} else if err.Error() != "policy returned incorrect type for 'env_list', expected []interface{}, received map[string]interface {}" {
pkg/securitypolicy/regopolicy_linux_test.go:3010:	} else if err.Error() != "policy returned incorrect type for 'env_list', expected []interface{}, received string" {
pkg/securitypolicy/regopolicy_linux_test.go:3017:	} else if err.Error() != "policy returned incorrect type for 'env_list', expected []interface{}, received bool" {
pkg/securitypolicy/regopolicy_linux_test.go:3054:	} else if err.Error() != "members of env_list from policy must be strings, received json.Number" {
pkg/securitypolicy/regopolicy_linux_test.go:3061:	} else if err.Error() != "members of env_list from policy must be strings, received bool" {
pkg/securitypolicy/regopolicy_linux_test.go:3068:	} else if err.Error() != "members of env_list from policy must be strings, received []interface {}" {
pkg/securitypolicy/regopolicy_linux_test.go:3087:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:3118:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:3127:			t.Error("Policy enforcement unexpectedly was denied")
pkg/securitypolicy/regopolicy_linux_test.go:3143:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:3152:			t.Error("Policy was unexpectedly not enforced")
pkg/securitypolicy/regopolicy_linux_test.go:3168:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:3178:			t.Error("Policy was unexpectedly not enforced")
pkg/securitypolicy/regopolicy_linux_test.go:3194:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:3203:			t.Error("Policy was unexpectedly not enforced")
pkg/securitypolicy/regopolicy_linux_test.go:3219:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:3229:			t.Error("Policy was unexpectedly not enforced")
pkg/securitypolicy/regopolicy_linux_test.go:3246:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:3318:		t.Error("environment variables were not dropped correctly.")
pkg/securitypolicy/regopolicy_linux_test.go:3365:		t.Error("expected container creation to not be allowed.")
pkg/securitypolicy/regopolicy_linux_test.go:3369:		t.Error("envList should be nil.")
pkg/securitypolicy/regopolicy_linux_test.go:3413:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:3452:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:3490:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:3533:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:3588:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:3643:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:3699:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:3909:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:3915:			t.Error("Policy enforcement unexpectedly was denied")
pkg/securitypolicy/regopolicy_linux_test.go:3931:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:3937:			t.Error("Policy enforcement unexpectedly was allowed")
pkg/securitypolicy/regopolicy_linux_test.go:3953:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:3975:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:3981:			t.Error("Policy enforcement unexpectedly was allowed")
pkg/securitypolicy/regopolicy_linux_test.go:4027:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:4036:			t.Error("unable to load fragment: %w", err)
pkg/securitypolicy/regopolicy_linux_test.go:4042:			t.Error("unable to mount image for fragment container: %w", err)
pkg/securitypolicy/regopolicy_linux_test.go:4063:			t.Error("unable to create container from fragment: %w", err)
pkg/securitypolicy/regopolicy_linux_test.go:4068:			t.Error("module not removed after load")
pkg/securitypolicy/regopolicy_linux_test.go:4085:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:4100:			t.Error("unable to load fragment: %w", err)
pkg/securitypolicy/regopolicy_linux_test.go:4106:			t.Error("unable to mount image for fragment container: %w", err)
pkg/securitypolicy/regopolicy_linux_test.go:4127:			t.Error("unable to create container from fragment: %w", err)
pkg/securitypolicy/regopolicy_linux_test.go:4132:			t.Error("module not removed after load")
pkg/securitypolicy/regopolicy_linux_test.go:4149:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:4164:			t.Error("unable to load fragment: %w", err)
pkg/securitypolicy/regopolicy_linux_test.go:4170:			t.Error("unable to mount image for fragment container: %w", err)
pkg/securitypolicy/regopolicy_linux_test.go:4191:			t.Error("unable to create container from fragment: %w", err)
pkg/securitypolicy/regopolicy_linux_test.go:4196:			t.Error("module not removed after load")
pkg/securitypolicy/regopolicy_linux_test.go:4212:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:4221:			t.Error("unable to load fragment: %w", err)
pkg/securitypolicy/regopolicy_linux_test.go:4227:			t.Error("unable to load sub-fragment from fragment: %w", err)
pkg/securitypolicy/regopolicy_linux_test.go:4234:			t.Error("unable to mount image for sub-fragment container: %w", err)
pkg/securitypolicy/regopolicy_linux_test.go:4239:			t.Error("module not removed after load")
pkg/securitypolicy/regopolicy_linux_test.go:4255:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:4264:			t.Error("unable to load fragment: %w", err)
pkg/securitypolicy/regopolicy_linux_test.go:4271:			t.Error("unable to execute external process from fragment: %w", err)
pkg/securitypolicy/regopolicy_linux_test.go:4276:			t.Error("module not removed after load")
pkg/securitypolicy/regopolicy_linux_test.go:4292:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:4300:			t.Error("expected to be unable to load fragment due to bad issuer")
pkg/securitypolicy/regopolicy_linux_test.go:4305:			t.Error("expected error string to contain 'invalid fragment issuer'")
pkg/securitypolicy/regopolicy_linux_test.go:4310:			t.Error("module not removed upon failure")
pkg/securitypolicy/regopolicy_linux_test.go:4326:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:4334:			t.Error("expected to be unable to load fragment due to bad feed")
pkg/securitypolicy/regopolicy_linux_test.go:4339:			t.Error("expected error string to contain 'invalid fragment feed'")
pkg/securitypolicy/regopolicy_linux_test.go:4344:			t.Error("module not removed upon failure")
pkg/securitypolicy/regopolicy_linux_test.go:4420:	if err != nil && !strings.Contains(err.Error(), "value not found") &&
pkg/securitypolicy/regopolicy_linux_test.go:4421:		!strings.Contains(err.Error(), "metadata not found for name issuers") {
pkg/securitypolicy/regopolicy_linux_test.go:4436:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:4460:			t.Error("expected to be unable to load fragment due to bad namespace")
pkg/securitypolicy/regopolicy_linux_test.go:4464:		if !strings.Contains(err.Error(), "namespace \"framework\" is reserved") {
pkg/securitypolicy/regopolicy_linux_test.go:4465:			t.Errorf("expected error string to contain 'namespace \"framework\" is reserved', but got %q", err.Error())
pkg/securitypolicy/regopolicy_linux_test.go:4485:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:4502:			t.Error("expected to be unable to load fragment due to invalid namespace")
pkg/securitypolicy/regopolicy_linux_test.go:4506:		if !strings.Contains(err.Error(), "valid package definition required on first line") {
pkg/securitypolicy/regopolicy_linux_test.go:4507:			t.Errorf("expected error string to contain 'valid package definition required on first line', but got %q", err.Error())
pkg/securitypolicy/regopolicy_linux_test.go:4527:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:4534:			t.Error("expected to be unable to load fragment due to invalid svn")
pkg/securitypolicy/regopolicy_linux_test.go:4539:			t.Error("expected error string to contain 'fragment svn is below the specified minimum'")
pkg/securitypolicy/regopolicy_linux_test.go:4544:			t.Error("module not removed upon failure")
pkg/securitypolicy/regopolicy_linux_test.go:4560:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:4567:			t.Error("unable to load fragment: %w", err)
pkg/securitypolicy/regopolicy_linux_test.go:4574:			t.Error("expected to be unable to load subfragment due to invalid svn")
pkg/securitypolicy/regopolicy_linux_test.go:4579:			t.Error("expected error string to contain 'fragment svn is below the specified minimum'")
pkg/securitypolicy/regopolicy_linux_test.go:4584:			t.Error("module not removed upon failure")
pkg/securitypolicy/regopolicy_linux_test.go:4619:			t.Error("unable to load fragment: %w", err)
pkg/securitypolicy/regopolicy_linux_test.go:4624:			t.Error("module not removed after load")
pkg/securitypolicy/regopolicy_linux_test.go:4640:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:4647:			t.Error("expected to be unable to load fragment due to invalid version")
pkg/securitypolicy/regopolicy_linux_test.go:4652:			t.Error("expected error string to contain 'fragment svn and the specified minimum are different types'")
pkg/securitypolicy/regopolicy_linux_test.go:4657:			t.Error("module not removed upon failure")
pkg/securitypolicy/regopolicy_linux_test.go:4673:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:4680:				t.Error("unable to load fragment: %w", err)
pkg/securitypolicy/regopolicy_linux_test.go:4688:				t.Error("unable to mount image for fragment container: %w", err)
pkg/securitypolicy/regopolicy_linux_test.go:4709:				t.Error("unable to create container from fragment: %w", err)
pkg/securitypolicy/regopolicy_linux_test.go:4726:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:4733:				t.Error("unable to load fragment: %w", err)
pkg/securitypolicy/regopolicy_linux_test.go:4741:				t.Error("unable to mount image for fragment container: %w", err)
pkg/securitypolicy/regopolicy_linux_test.go:4762:				t.Error("unable to create container from fragment: %w", err)
pkg/securitypolicy/regopolicy_linux_test.go:4779:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:4785:			t.Error("unable to load fragment the first time: %w", err)
pkg/securitypolicy/regopolicy_linux_test.go:4791:			t.Error("expected to be able to load the same issuer/feed twice: %w", err)
pkg/securitypolicy/regopolicy_linux_test.go:4798:				t.Error("unable to mount image for fragment container: %w", err)
pkg/securitypolicy/regopolicy_linux_test.go:4819:				t.Error("unable to create container from fragment: %w", err)
pkg/securitypolicy/regopolicy_linux_test.go:4836:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:4845:			t.Error("unable to load fragment: %w", err)
pkg/securitypolicy/regopolicy_linux_test.go:4851:			t.Error("expected to be unable to mount image for fragment container")
pkg/securitypolicy/regopolicy_linux_test.go:4867:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:4876:			t.Error("unable to load fragment: %w", err)
pkg/securitypolicy/regopolicy_linux_test.go:4882:			t.Error("expected to be unable to load a sub-fragment from a fragment")
pkg/securitypolicy/regopolicy_linux_test.go:4898:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:4907:			t.Error("unable to load fragment: %w", err)
pkg/securitypolicy/regopolicy_linux_test.go:4915:			t.Error("expected to be unable to execute external process from a fragment")
pkg/securitypolicy/regopolicy_linux_test.go:4991:			t.Error("incorrect metadata value stored by fragment")
pkg/securitypolicy/regopolicy_linux_test.go:5002:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:5048:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:5092:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:5102:		if strings.Contains(err.Error(), "error when compiling module") ||
pkg/securitypolicy/regopolicy_linux_test.go:5104:			t.Errorf("expected error to not contain 'error when compiling module', got: %s", err.Error())
pkg/securitypolicy/regopolicy_linux_test.go:5128:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:5138:		if strings.Contains(err.Error(), "error when compiling module") ||
pkg/securitypolicy/regopolicy_linux_test.go:5140:			t.Errorf("expected error to not contain 'error when compiling module', got: %s", err.Error())
pkg/securitypolicy/regopolicy_linux_test.go:5164:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:5180:		if strings.Contains(err.Error(), "error when compiling module") ||
pkg/securitypolicy/regopolicy_linux_test.go:5182:			t.Errorf("expected error to not contain 'error when compiling module', got: %s", err.Error())
pkg/securitypolicy/regopolicy_linux_test.go:5650:		t.Error("incorrect number of candidate containers.")
pkg/securitypolicy/regopolicy_linux_test.go:5657:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:5682:		t.Error("incorrect number of candidate containers.")
pkg/securitypolicy/regopolicy_linux_test.go:5689:			t.Error("unable to extract user from container")
pkg/securitypolicy/regopolicy_linux_test.go:5695:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:5701:				t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:5704:			t.Error("unable to extract user_idname from user")
pkg/securitypolicy/regopolicy_linux_test.go:5709:				t.Error("incorrect number of group_idnames")
pkg/securitypolicy/regopolicy_linux_test.go:5714:					t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:5743:		t.Error("incorrect number of candidate containers.")
pkg/securitypolicy/regopolicy_linux_test.go:5750:			t.Error("unable to extract capabilities from container")
pkg/securitypolicy/regopolicy_linux_test.go:5755:			t.Error("capabilities should be nil by default")
pkg/securitypolicy/regopolicy_linux_test.go:5780:		t.Error("unexpected allow_capability_dropping value")
pkg/securitypolicy/regopolicy_linux_test.go:5804:		t.Error("incorrect number of candidate containers.")
pkg/securitypolicy/regopolicy_linux_test.go:5811:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:5830:		t.Error("unexpected success. Missing framework_version should trigger an error.")
pkg/securitypolicy/regopolicy_linux_test.go:5846:		t.Error("unexpected success. Missing framework_version should trigger an error.")
pkg/securitypolicy/regopolicy_linux_test.go:5867:		t.Error("unexpected success. Future framework_version should trigger an error.")
pkg/securitypolicy/regopolicy_linux_test.go:5883:		t.Error("unexpected success. Future framework_version should trigger an error.")
pkg/securitypolicy/regopolicy_linux_test.go:5941:		t.Error("invalid envList returned from EnforceCreateContainerPolicy")
pkg/securitypolicy/regopolicy_linux_test.go:5951:		t.Error("invalid envList returned from EnforceExecInContainerPolicy")
pkg/securitypolicy/regopolicy_linux_test.go:5961:		t.Error("invalid envList returned from EnforceExecExternalProcessPolicy")
pkg/securitypolicy/regopolicy_linux_test.go:6147:		fmt.Print(err.Error())
pkg/securitypolicy/regopolicy_linux_test.go:6191:		fmt.Print(err.Error())
pkg/securitypolicy/regopolicy_linux_test.go:6368:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:6392:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:6416:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:6440:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:6472:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:6505:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:6631:		t.Error("Expected container setup to not be allowed.")
pkg/securitypolicy/regopolicy_linux_test.go:6633:		t.Error("`invalid seccomp` missing from error message")
pkg/securitypolicy/regopolicy_linux_test.go:6662:			t.Error("policy_framework_version is not set correctly from framework_svn")
pkg/securitypolicy/regopolicy_linux_test.go:6665:		t.Error("no result set from querying data.framework.policy_framework_version")
pkg/securitypolicy/regopolicy_linux_test.go:6709:			t.Error("fragment_framework_version is not set correctly from framework_svn")
pkg/securitypolicy/regopolicy_linux_test.go:6712:		t.Error("no result set from querying data.framework.fragment_framework_version")
pkg/securitypolicy/regopolicy_linux_test.go:6741:			t.Error("policy_api_version is not set correctly from api_svn")
pkg/securitypolicy/regopolicy_linux_test.go:6744:		t.Error("no result set from querying data.framework.policy_api_version")
pkg/securitypolicy/regopolicy_linux_test.go:6764:		t.Error("expected error, got nil")
pkg/securitypolicy/regopolicy_linux_test.go:6774:			t.Error(err)
pkg/securitypolicy/regopolicy_linux_test.go:6787:		if len(err.Error()) > maxErrorMessageLength {
pkg/securitypolicy/regopolicy_linux_test.go:6791:		policyDecisionJSON, err := ExtractPolicyDecision(err.Error())
pkg/securitypolicy/regopolicy_linux_test.go:6805:				t.Error("first item to be truncated should be reason.error_objects")
pkg/securitypolicy/regopolicy_linux_test.go:6808:				t.Error("second item to be truncated should be input")
pkg/securitypolicy/regopolicy_linux_test.go:6860:		t.Error("expected error, got nil")
pkg/securitypolicy/regopolicy_linux_test.go:7141:				testGetUserInfo(t, tc, userStr, regoEnforcer, testDir)
pkg/securitypolicy/regopolicy_linux_test.go:7209:				testGetUserInfo(t, tc, userStr, regoEnforcer, testDir)
pkg/securitypolicy/regopolicy_linux_test.go:7277:				testGetUserInfo(t, tc, userStr, regoEnforcer, testDir)
pkg/securitypolicy/regopolicy_linux_test.go:7293:func testGetUserInfo(t *testing.T, tc getUserInfoTestCase, userStr string, regoEnforcer *regoEnforcer, testDir string) {
pkg/securitypolicy/regopolicy_linux_test.go:7317:		userIDName, groupIDNames, umask, err := regoEnforcer.GetUserInfo(ociProcess, testDir)
pkg/securitypolicy/regopolicy_windows_test.go:27:			t.Error(err)
pkg/securitypolicy/regopolicy_windows_test.go:62:			t.Error(err)
pkg/securitypolicy/regopolicy_windows_test.go:88:			t.Error(err)
pkg/securitypolicy/regopolicy_windows_test.go:116:			t.Error(err)
pkg/securitypolicy/regopolicy_windows_test.go:163:		t.Error("environment variables were not dropped correctly.")
pkg/securitypolicy/regopolicy_windows_test.go:181:		t.Error("expected container creation not to be allowed.")
pkg/securitypolicy/regopolicy_windows_test.go:185:		t.Error("envList should be nil")
pkg/securitypolicy/regopolicy_windows_test.go:193:			t.Error(err)
pkg/securitypolicy/regopolicy_windows_test.go:242:			t.Error(err)
pkg/securitypolicy/regopolicy_windows_test.go:249:				t.Error(err)
pkg/securitypolicy/regopolicy_windows_test.go:260:				t.Error(err)
pkg/securitypolicy/regopolicy_windows_test.go:276:			t.Error(err)
pkg/securitypolicy/regopolicy_windows_test.go:296:			t.Error(err)
pkg/securitypolicy/regopolicy_windows_test.go:302:			t.Error("Unable to start valid container.")
pkg/securitypolicy/regopolicy_windows_test.go:308:			t.Error("Able to start a container with already used id.")
pkg/securitypolicy/regopolicy_windows_test.go:364:			t.Error(err)
pkg/securitypolicy/regopolicy_windows_test.go:380:			t.Error(err)
pkg/securitypolicy/regopolicy_windows_test.go:396:			t.Error(err)
pkg/securitypolicy/regopolicy_windows_test.go:408:			t.Error("Test unexpectedly passed")
pkg/securitypolicy/regopolicy_windows_test.go:424:			t.Error(err)
pkg/securitypolicy/regopolicy_windows_test.go:437:			t.Error("Unexpected success when enforcing policy")
pkg/securitypolicy/regopolicy_windows_test.go:453:			t.Error(err)
pkg/securitypolicy/regopolicy_windows_test.go:466:			t.Error("Unexpected success when enforcing policy")
pkg/securitypolicy/regopolicy_windows_test.go:482:			t.Error(err)
pkg/securitypolicy/regopolicy_windows_test.go:497:			t.Error("Unexpected success when enforcing policy")
pkg/securitypolicy/regopolicy_windows_test.go:515:			t.Error(err)
pkg/securitypolicy/regopolicy_windows_test.go:597:			t.Error("invalid environment variables not filtered from list returned from create_container")
pkg/securitypolicy/regopolicy_windows_test.go:604:			t.Error("invalid environment variables not filtered from list returned from exec_in_container")
pkg/securitypolicy/regopolicy_windows_test.go:611:			t.Error("invalid environment variables not filtered from list returned from exec_external")
pkg/securitypolicy/regopolicy_windows_test.go:653:	} else if err.Error() != "policy returned incorrect type for 'env_list', expected []interface{}, received map[string]interface {}" {
pkg/securitypolicy/regopolicy_windows_test.go:660:	} else if err.Error() != "policy returned incorrect type for 'env_list', expected []interface{}, received string" {
pkg/securitypolicy/regopolicy_windows_test.go:667:	} else if err.Error() != "policy returned incorrect type for 'env_list', expected []interface{}, received bool" {
pkg/securitypolicy/regopolicy_windows_test.go:702:	} else if err.Error() != "members of env_list from policy must be strings, received json.Number" {
pkg/securitypolicy/regopolicy_windows_test.go:709:	} else if err.Error() != "members of env_list from policy must be strings, received bool" {
pkg/securitypolicy/regopolicy_windows_test.go:716:	} else if err.Error() != "members of env_list from policy must be strings, received []interface {}" {
pkg/securitypolicy/regopolicy_windows_test.go:735:			t.Error(err)
pkg/securitypolicy/regopolicy_windows_test.go:766:			t.Error(err)
pkg/securitypolicy/regopolicy_windows_test.go:775:			t.Error("Policy enforcement unexpectedly was denied")
pkg/securitypolicy/regopolicy_windows_test.go:791:			t.Error(err)
pkg/securitypolicy/regopolicy_windows_test.go:800:			t.Error("Policy was unexpectedly not enforced")
pkg/securitypolicy/regopolicy_windows_test.go:816:			t.Error(err)
pkg/securitypolicy/regopolicy_windows_test.go:826:			t.Error("Policy was unexpectedly not enforced")
pkg/securitypolicy/regopolicy_windows_test.go:842:			t.Error(err)
pkg/securitypolicy/regopolicy_windows_test.go:851:			t.Error("Policy was unexpectedly not enforced")
pkg/securitypolicy/regopolicy_windows_test.go:867:			t.Error(err)
pkg/securitypolicy/regopolicy_windows_test.go:877:			t.Error("Policy was unexpectedly not enforced")
pkg/securitypolicy/regopolicy_windows_test.go:894:			t.Error(err)
pkg/securitypolicy/regopolicy_windows_test.go:966:		t.Error("environment variables were not dropped correctly.")
pkg/securitypolicy/regopolicy_windows_test.go:1013:		t.Error("expected container creation to not be allowed.")
pkg/securitypolicy/regopolicy_windows_test.go:1017:		t.Error("envList should be nil.")
pkg/securitypolicy/regopolicy_windows_test.go:1066:			t.Error(err)
pkg/securitypolicy/regopolicy_windows_test.go:1123:			t.Error(err)
pkg/securitypolicy/regopolicy_windows_test.go:1181:			t.Error(err)
pkg/securitypolicy/regopolicy_windows_test.go:1240:			t.Error(err)
pkg/securitypolicy/regopolicy_windows_test.go:1290:			t.Error(err)
pkg/securitypolicy/regopolicy_windows_test.go:1296:			t.Error("Policy enforcement unexpectedly was denied")
pkg/securitypolicy/regopolicy_windows_test.go:1312:			t.Error(err)
pkg/securitypolicy/regopolicy_windows_test.go:1318:			t.Error("Policy enforcement unexpectedly was allowed")
pkg/securitypolicy/regopolicy_windows_test.go:1334:			t.Error(err)
pkg/securitypolicy/regopolicy_windows_test.go:1356:			t.Error(err)
pkg/securitypolicy/regopolicy_windows_test.go:1362:			t.Error("Policy enforcement unexpectedly was allowed")
pkg/securitypolicy/regopolicy_windows_test.go:1379:			t.Error(err)
pkg/securitypolicy/regopolicy_windows_test.go:1419:			t.Error(err)
pkg/securitypolicy/regopolicy_windows_test.go:1442:			t.Error("Expected registry changes to be denied with invalid container ID")
pkg/securitypolicy/regopolicy_windows_test.go:1459:			t.Error(err)
pkg/securitypolicy/securitypolicy_linux.go:118:func GetAllUserInfo(process *oci.Process, rootPath string) (
pkg/securitypolicy/securitypolicy_options.go:113:	log.G(ctx).WithField("fragment", fmt.Sprintf("%+v", fragment)).Debug("VerifyAndExtractFragment")
pkg/securitypolicy/securitypolicy_options.go:158:		log.G(ctx).Printf("Badly formed fragment - did resolver failed to match fragment did:x509 from chain with purported issuer %s, feed %s - err %s", issuer, feed, err.Error())
pkg/securitypolicy/securitypolicy_windows.go:23:func GetAllUserInfo(process *oci.Process, rootPath string) (IDName, []IDName, string, error) {
pkg/securitypolicy/securitypolicyenforcer.go:128:	GetUserInfo(spec *oci.Process, rootPath string) (IDName, []IDName, string, error)
pkg/securitypolicy/securitypolicyenforcer.go:315:func (OpenDoorSecurityPolicyEnforcer) GetUserInfo(spec *oci.Process, rootPath string) (IDName, []IDName, string, error) {
pkg/securitypolicy/securitypolicyenforcer.go:444:func (ClosedDoorSecurityPolicyEnforcer) GetUserInfo(spec *oci.Process, rootPath string) (IDName, []IDName, string, error) {
pkg/securitypolicy/securitypolicyenforcer_rego.go:270:		return nil, policy.denyWithError(ctx, err, input)
pkg/securitypolicy/securitypolicyenforcer_rego.go:275:		return result, policy.denyWithError(ctx, err, input)
pkg/securitypolicy/securitypolicyenforcer_rego.go:280:		return nil, policy.denyWithError(ctx, err, input)
pkg/securitypolicy/securitypolicyenforcer_rego.go:319:func (policy *regoEnforcer) policyDecisionToError(ctx context.Context, decision map[string]interface{}) error {
pkg/securitypolicy/securitypolicyenforcer_rego.go:322:		log.G(ctx).WithError(err).Error("unable to marshal error object")
pkg/securitypolicy/securitypolicyenforcer_rego.go:335:	if len(errorMessage.Error()) <= policy.maxErrorMessageLength {
pkg/securitypolicy/securitypolicyenforcer_rego.go:346:			log.G(ctx).WithError(err).Error("unable to marshal error object")
pkg/securitypolicy/securitypolicyenforcer_rego.go:352:		if len(errorMessage.Error()) <= policy.maxErrorMessageLength {
pkg/securitypolicy/securitypolicyenforcer_rego.go:360:func (policy *regoEnforcer) denyWithError(ctx context.Context, policyError error, input inputData) error {
pkg/securitypolicy/securitypolicyenforcer_rego.go:367:		"policyError": policyError.Error(),
pkg/securitypolicy/securitypolicyenforcer_rego.go:370:	return policy.policyDecisionToError(ctx, policyDecision)
pkg/securitypolicy/securitypolicyenforcer_rego.go:390:		log.G(ctx).WithError(err).Warn("unable to obtain reason for policy decision")
pkg/securitypolicy/securitypolicyenforcer_rego.go:394:	return policy.policyDecisionToError(ctx, policyDecision)
pkg/securitypolicy/securitypolicyenforcer_rego.go:763:			log.G(ctx).WithError(err).Warn("failed to obtain policy metadata snapshot")
pkg/securitypolicy/securitypolicyenforcer_rego.go:1177:		log.G(ctx).Warn("Input registry values are not of expected type")
pkg/securitypolicy/securitypolicyenforcer_rego.go:1190:func (policy *regoEnforcer) GetUserInfo(process *oci.Process, rootPath string) (IDName, []IDName, string, error) {
pkg/securitypolicy/securitypolicyenforcer_rego.go:1191:	return GetAllUserInfo(process, rootPath)
```

# Package Inventory

```txt
github.com/Microsoft/hcsshim
github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options
github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/stats
github.com/Microsoft/hcsshim/cmd/gcs
github.com/Microsoft/hcsshim/cmd/gcstools
github.com/Microsoft/hcsshim/cmd/gcstools/commoncli
github.com/Microsoft/hcsshim/cmd/gcstools/generichook
github.com/Microsoft/hcsshim/cmd/hooks/wait-paths
github.com/Microsoft/hcsshim/cmd/tar2ext4
github.com/Microsoft/hcsshim/computestorage
github.com/Microsoft/hcsshim/ext4/dmverity
github.com/Microsoft/hcsshim/ext4/internal/compactext4
github.com/Microsoft/hcsshim/ext4/internal/format
github.com/Microsoft/hcsshim/ext4/tar2ext4
github.com/Microsoft/hcsshim/hcn
github.com/Microsoft/hcsshim/internal/annotations
github.com/Microsoft/hcsshim/internal/appargs
github.com/Microsoft/hcsshim/internal/bridgeutils/commonutils
github.com/Microsoft/hcsshim/internal/bridgeutils/gcserr
github.com/Microsoft/hcsshim/internal/builder/vm/lcow
github.com/Microsoft/hcsshim/internal/cmd
github.com/Microsoft/hcsshim/internal/cni
github.com/Microsoft/hcsshim/internal/computeagent
github.com/Microsoft/hcsshim/internal/computeagent/mock
github.com/Microsoft/hcsshim/internal/computecore
github.com/Microsoft/hcsshim/internal/conpty
github.com/Microsoft/hcsshim/internal/copyfile
github.com/Microsoft/hcsshim/internal/cpugroup
github.com/Microsoft/hcsshim/internal/credentials
github.com/Microsoft/hcsshim/internal/debug
github.com/Microsoft/hcsshim/internal/devices
github.com/Microsoft/hcsshim/internal/exec
github.com/Microsoft/hcsshim/internal/extendedtask
github.com/Microsoft/hcsshim/internal/gcs
github.com/Microsoft/hcsshim/internal/guest/bridge
github.com/Microsoft/hcsshim/internal/guest/kmsg
github.com/Microsoft/hcsshim/internal/guest/linux
github.com/Microsoft/hcsshim/internal/guest/network
github.com/Microsoft/hcsshim/internal/guest/prot
github.com/Microsoft/hcsshim/internal/guest/runtime
github.com/Microsoft/hcsshim/internal/guest/runtime/hcsv2
github.com/Microsoft/hcsshim/internal/guest/runtime/runc
github.com/Microsoft/hcsshim/internal/guest/spec
github.com/Microsoft/hcsshim/internal/guest/stdio
github.com/Microsoft/hcsshim/internal/guest/storage
github.com/Microsoft/hcsshim/internal/guest/storage/crypt
github.com/Microsoft/hcsshim/internal/guest/storage/devicemapper
github.com/Microsoft/hcsshim/internal/guest/storage/ext4
github.com/Microsoft/hcsshim/internal/guest/storage/overlay
github.com/Microsoft/hcsshim/internal/guest/storage/pci
github.com/Microsoft/hcsshim/internal/guest/storage/plan9
github.com/Microsoft/hcsshim/internal/guest/storage/pmem
github.com/Microsoft/hcsshim/internal/guest/storage/scsi
github.com/Microsoft/hcsshim/internal/guest/storage/vmbus
github.com/Microsoft/hcsshim/internal/guest/storage/xfs
github.com/Microsoft/hcsshim/internal/guest/transport
github.com/Microsoft/hcsshim/internal/guestpath
github.com/Microsoft/hcsshim/internal/hcs
github.com/Microsoft/hcsshim/internal/hcs/resourcepaths
github.com/Microsoft/hcsshim/internal/hcs/schema2
github.com/Microsoft/hcsshim/internal/hcserror
github.com/Microsoft/hcsshim/internal/hcsoci
github.com/Microsoft/hcsshim/internal/hns
github.com/Microsoft/hcsshim/internal/hooks
github.com/Microsoft/hcsshim/internal/interop
github.com/Microsoft/hcsshim/internal/jobcontainers
github.com/Microsoft/hcsshim/internal/jobobject
github.com/Microsoft/hcsshim/internal/layers
github.com/Microsoft/hcsshim/internal/lcow
github.com/Microsoft/hcsshim/internal/log
github.com/Microsoft/hcsshim/internal/logfields
github.com/Microsoft/hcsshim/internal/longpath
github.com/Microsoft/hcsshim/internal/memory
github.com/Microsoft/hcsshim/internal/mergemaps
github.com/Microsoft/hcsshim/internal/ncproxy/networking
github.com/Microsoft/hcsshim/internal/ncproxy/store
github.com/Microsoft/hcsshim/internal/ncproxyttrpc
github.com/Microsoft/hcsshim/internal/oc
github.com/Microsoft/hcsshim/internal/oci
github.com/Microsoft/hcsshim/internal/ospath
github.com/Microsoft/hcsshim/internal/processorinfo
github.com/Microsoft/hcsshim/internal/protocol/guestrequest
github.com/Microsoft/hcsshim/internal/protocol/guestresource
github.com/Microsoft/hcsshim/internal/queue
github.com/Microsoft/hcsshim/internal/regopolicyinterpreter
github.com/Microsoft/hcsshim/internal/regstate
github.com/Microsoft/hcsshim/internal/resources
github.com/Microsoft/hcsshim/internal/runhcs
github.com/Microsoft/hcsshim/internal/safefile
github.com/Microsoft/hcsshim/internal/schemaversion
github.com/Microsoft/hcsshim/internal/shimdiag
github.com/Microsoft/hcsshim/internal/signals
github.com/Microsoft/hcsshim/internal/timeout
github.com/Microsoft/hcsshim/internal/tools/hvsocketaddr
github.com/Microsoft/hcsshim/internal/tools/policyenginesimulator
github.com/Microsoft/hcsshim/internal/tools/rootfs
github.com/Microsoft/hcsshim/internal/tools/securitypolicy
github.com/Microsoft/hcsshim/internal/tools/securitypolicy/helpers
github.com/Microsoft/hcsshim/internal/tools/snp-report
github.com/Microsoft/hcsshim/internal/tools/snp-report/fake
github.com/Microsoft/hcsshim/internal/uvm
github.com/Microsoft/hcsshim/internal/uvm/scsi
github.com/Microsoft/hcsshim/internal/uvmfolder
github.com/Microsoft/hcsshim/internal/verity
github.com/Microsoft/hcsshim/internal/version
github.com/Microsoft/hcsshim/internal/vhdx
github.com/Microsoft/hcsshim/internal/vm/vmutils/etw
github.com/Microsoft/hcsshim/internal/vmcompute
github.com/Microsoft/hcsshim/internal/wclayer
github.com/Microsoft/hcsshim/internal/wclayer/cim
github.com/Microsoft/hcsshim/internal/wcow
github.com/Microsoft/hcsshim/internal/winapi
github.com/Microsoft/hcsshim/internal/winapi/cimfs
github.com/Microsoft/hcsshim/internal/winapi/cimwriter
github.com/Microsoft/hcsshim/internal/windevice
github.com/Microsoft/hcsshim/internal/winobjdir
github.com/Microsoft/hcsshim/osversion
github.com/Microsoft/hcsshim/pkg/amdsevsnp
github.com/Microsoft/hcsshim/pkg/annotations
github.com/Microsoft/hcsshim/pkg/cimfs
github.com/Microsoft/hcsshim/pkg/cimfs/format
github.com/Microsoft/hcsshim/pkg/ctrdtaskapi
github.com/Microsoft/hcsshim/pkg/go-runhcs
github.com/Microsoft/hcsshim/pkg/ncproxy/ncproxygrpc/v0
github.com/Microsoft/hcsshim/pkg/ncproxy/ncproxygrpc/v1
github.com/Microsoft/hcsshim/pkg/ncproxy/nodenetsvc/v0
github.com/Microsoft/hcsshim/pkg/ncproxy/nodenetsvc/v0/mock
github.com/Microsoft/hcsshim/pkg/ncproxy/nodenetsvc/v1
github.com/Microsoft/hcsshim/pkg/ncproxy/nodenetsvc/v1/mock
github.com/Microsoft/hcsshim/pkg/ociwclayer
github.com/Microsoft/hcsshim/pkg/octtrpc
github.com/Microsoft/hcsshim/pkg/securitypolicy
github.com/Microsoft/hcsshim/sandbox-spec/vm/v2
```

# Migration Planning Template

| Package | Coupling Type | Risk | Notes | Migration Strategy |
|---|---|---|---|---|
| internal/log | wrapper | low | existing abstraction | replace backend |
| internal/hcs | structured fields | medium | many WithField usages | mechanical migration |
| internal/uvm | typed logger propagation | high | logrus.Entry in structs | adapter first |
