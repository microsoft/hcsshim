# hcsshim Logging Abstraction Audit
Generated: 2026-05-18T17:17:28.650994 UTC

# Executive Summary

| Metric | Count |
|---|---|
| Direct logrus Imports | 197 |
| Structured Logging Usage | 902 |
| logrus.Entry Usage | 35 |
| Formatter / Hook Usage | 143 |
| Context Logger Usage | 640 |
| Public API Exposure | 24 |

# Migration Strategy Recommendation


Recommended phased approach:

1. Introduce internal logging abstraction
2. Add logrus compatibility adapter
3. Preserve all existing behavior
4. Migrate internal packages incrementally
5. Add optional slog/zap adapters later

Avoid:
- mass rewrites
- changing log formats
- changing field semantics
- breaking public APIs


# Direct logrus Imports

Severity: **high**

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

# Structured Logging Usage

Severity: **medium**

```txt
cmd/containerd-shim-lcow-v2/manager.go:211:		logrus.WithField("log", string(logBytes)).Warn("found shim panic logs during delete")
cmd/containerd-shim-lcow-v2/manager.go:213:		logrus.WithError(err).Warn("failed to open shim panic log")
cmd/containerd-shim-lcow-v2/service/plugin/plugin.go:50:		etw.WithFields(
cmd/containerd-shim-lcow-v2/service/plugin/plugin.go:109:		log := logrus.WithField("sandboxID", svc.SandboxID())
cmd/containerd-shim-lcow-v2/service/plugin/plugin.go:110:		log.WithField("stack", resp.Stacks).Info("goroutine stack dump")
cmd/containerd-shim-lcow-v2/service/plugin/plugin.go:112:			log.WithField("stack", resp.GuestStacks).Info("guest stack dump")
cmd/containerd-shim-lcow-v2/service/service.go:116:			log.G(ctx).WithError(err).Error("post event")
cmd/containerd-shim-lcow-v2/service/service_sandbox.go:37:	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.SandboxID, request.SandboxID))
cmd/containerd-shim-lcow-v2/service/service_sandbox.go:53:	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.SandboxID, request.SandboxID))
cmd/containerd-shim-lcow-v2/service/service_sandbox.go:83:	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.SandboxID, request.SandboxID))
cmd/containerd-shim-lcow-v2/service/service_sandbox.go:99:	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.SandboxID, request.SandboxID))
cmd/containerd-shim-lcow-v2/service/service_sandbox.go:116:	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.SandboxID, request.SandboxID))
cmd/containerd-shim-lcow-v2/service/service_sandbox.go:132:	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.SandboxID, request.SandboxID))
cmd/containerd-shim-lcow-v2/service/service_sandbox.go:149:	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.SandboxID, request.SandboxID))
cmd/containerd-shim-lcow-v2/service/service_sandbox.go:166:	ctx, _ = log.WithContext(ctx, logrus.WithField(logfields.SandboxID, request.SandboxID))
cmd/containerd-shim-lcow-v2/service/service_sandbox_internal.go:276:			log.G(ctx).WithError(err).Error("failed to terminate VM during shutdown")
cmd/containerd-shim-runhcs-v1/delete.go:80:			logrus.WithField("log", string(logBytes)).Warn("found shim panic logs during delete")
cmd/containerd-shim-runhcs-v1/delete.go:82:			logrus.WithError(err).Warn("failed to open shim panic log")
cmd/containerd-shim-runhcs-v1/exec_hcs.go:46:	log.G(ctx).WithFields(logrus.Fields{
cmd/containerd-shim-runhcs-v1/exec_hcs.go:205:		Log: log.G(ctx).WithFields(logrus.Fields{
cmd/containerd-shim-runhcs-v1/exec_hcs.go:309:							log.G(ctx).WithField("err", deliveryErr).Errorf("Error in delivering signal %d, to pid: %d", signal, he.pid)
cmd/containerd-shim-runhcs-v1/exec_hcs.go:409:		log.G(ctx).WithField("status", status).Debug("hcsExec::exitFromCreatedL")
cmd/containerd-shim-runhcs-v1/exec_hcs.go:460:		log.G(ctx).WithError(err).Error("failed process Wait")
cmd/containerd-shim-runhcs-v1/exec_hcs.go:469:		log.G(ctx).WithError(err).Error("failed to get ExitCode")
cmd/containerd-shim-runhcs-v1/exec_hcs.go:471:		log.G(ctx).WithField("exitCode", code).Debug("exited")
cmd/containerd-shim-runhcs-v1/exec_hcs.go:498:			log.G(ctx).WithError(err).Error("failed to publish TaskExitEvent")
cmd/containerd-shim-runhcs-v1/exec_wcow_podsandbox.go:22:	log.G(ctx).WithFields(logrus.Fields{
cmd/containerd-shim-runhcs-v1/exec_wcow_podsandbox.go:197:		log.G(ctx).WithField("status", status).Debug("wcowPodSandboxExec::ForceExit")
cmd/containerd-shim-runhcs-v1/main.go:63:		log := logrus.WithField("tid", svc.tid)
cmd/containerd-shim-runhcs-v1/main.go:64:		log.WithField("stack", resp.Stacks).Info("goroutine stack dump")
cmd/containerd-shim-runhcs-v1/main.go:66:			log.WithField("stack", resp.GuestStacks).Info("guest stack dump")
cmd/containerd-shim-runhcs-v1/main.go:98:		etw.WithFields(
cmd/containerd-shim-runhcs-v1/pod.go:40:	log.G(ctx).WithField("options", log.Format(ctx, *wopts)).Debug("initialize WCOW boot files")
cmd/containerd-shim-runhcs-v1/pod.go:136:	log.G(ctx).WithField("tid", req.ID).Debug("createPod")
cmd/containerd-shim-runhcs-v1/serve.go:219:				logrus.WithError(err).Fatal("containerd-shim: ttrpc server failure")
cmd/containerd-shim-runhcs-v1/serve.go:324:	logrus.WithField("event", event).Info("Halting until signalled")
cmd/containerd-shim-runhcs-v1/service_internal.go:90:			entry.WithField("options", log.Format(ctx, shimOpts)).Debug("parsed runhcs runtime options")
cmd/containerd-shim-runhcs-v1/task_hcs.go:56:	log.G(ctx).WithField("tid", req.ID).Debug("newHcsStandaloneTask")
cmd/containerd-shim-runhcs-v1/task_hcs.go:191:	log.G(ctx).WithFields(logrus.Fields{
cmd/containerd-shim-runhcs-v1/task_hcs.go:436:				log.G(ctx).WithFields(logrus.Fields{
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
cmd/containerd-shim-runhcs-v1/task_wcow_podsandbox.go:39:	log.G(ctx).WithField("tid", id).Debug("newWcowPodSandboxTask")
cmd/containerd-shim-runhcs-v1/task_wcow_podsandbox.go:191:				log.G(ctx).WithError(err).Error("failed to cleanup networking for utility VM")
cmd/containerd-shim-runhcs-v1/task_wcow_podsandbox.go:195:				log.G(ctx).WithError(err).Error("failed host vm shutdown")
cmd/containerd-shim-runhcs-v1/task_wcow_podsandbox.go:211:			log.G(ctx).WithError(err).Error("failed to publish TaskExitEventTopic")
cmd/containerd-shim-runhcs-v1/task_wcow_podsandbox.go:236:		log.G(ctx).WithError(werr).Error("parent wait failed")
cmd/containerd-shim-runhcs-v1/task_wcow_podsandbox.go:268:			log.G(ctx).WithError(err).Warn("failed to capture guest stacks")
cmd/gcs-sidecar/main.go:160:		logrus.WithError(err).Error("error redirecting handle")
cmd/gcs-sidecar/main.go:192:		logrus.WithError(vsmbError).Errorf("VSMB redirector initialization failed.")
cmd/gcs-sidecar/main.go:201:		logrus.WithError(err).Error("error starting listener for sidecar <-> inbox gcs communication")
cmd/gcs-sidecar/main.go:208:		logrus.WithError(err).Error("error accepting inbox GCS connection")
cmd/gcs-sidecar/main.go:218:	logrus.WithFields(logrus.Fields{
cmd/gcs-sidecar/main.go:223:		logrus.WithError(err).Error("error dialing hcsshim external bridge")
cmd/gcs-sidecar/main.go:231:		logrus.WithError(err).Errorf("failed to start PSP driver")
cmd/gcs-sidecar/main.go:245:		logrus.WithError(err).Error("failed to serve request")
cmd/gcs/main.go:58:			logrus.WithError(err).WithField("cgroup", cgName).Error("failed to read from eventfd")
cmd/gcs/main.go:77:		entry := logrus.WithFields(logrus.Fields{
cmd/gcs/main.go:92:			entry.WithError(err).Error(msg)
cmd/gcs/main.go:94:			entry.WithFields(memoryLogFormat(metrics)).Warn(msg)
cmd/gcs/main.go:111:			logrus.WithFields(logrus.Fields{
cmd/gcs/main.go:231:			logrus.WithFields(logrus.Fields{
cmd/gcs/main.go:252:		logrus.WithFields(logrus.Fields{
cmd/gcs/main.go:265:		logrus.WithFields(logrus.Fields{
cmd/gcs/main.go:281:	logrus.WithFields(logrus.Fields{
cmd/gcs/main.go:296:			logrus.WithError(err).Fatal("failed to set core dump location")
cmd/gcs/main.go:312:		logrus.WithError(err).Fatal("failed to enable hierarchy support for root cgroup")
cmd/gcs/main.go:322:		logrus.WithError(err).Fatal("failed to get sys info")
cmd/gcs/main.go:331:		logrus.WithError(err).Fatal("failed to create containers cgroup")
cmd/gcs/main.go:343:		logrus.WithError(err).Fatal("failed to create containers/virtual-pods cgroup")
cmd/gcs/main.go:349:		logrus.WithError(err).Fatal("failed to create gcs cgroup")
cmd/gcs/main.go:353:		logrus.WithError(err).Fatal("failed add gcs pid to gcs cgroup")
cmd/gcs/main.go:359:		logrus.WithError(err).Fatal("failed to initialize new runc runtime")
cmd/gcs/main.go:380:			logrus.WithFields(logrus.Fields{
cmd/gcs/main.go:392:		logrus.WithError(err).Fatal("failed to register memory threshold for gcs cgroup")
cmd/gcs/main.go:399:		logrus.WithError(err).Fatal("failed to retrieve the container cgroups oom eventfd")
cmd/gcs/main.go:407:		logrus.WithError(err).Fatal("failed to retrieve the virtual-pods cgroups oom eventfd")
cmd/gcs/main.go:415:			logrus.WithError(err).Fatal("failed to start time synchronization service")
cmd/gcs/main.go:424:		logrus.WithFields(logrus.Fields{
cmd/gcstools/generichook.go:72:		logrus.WithField("output", output).Infof("%s debug part %d", debugFilePath, i)
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
cmd/ncproxy/run.go:90:		logrus.WithField("stack", stacks).Info("ncproxy goroutine stack dump")
cmd/ncproxy/run.go:277:	log.G(ctx).WithFields(logrus.Fields{
cmd/ncproxy/server.go:53:		log.G(ctx).WithError(err).Error("failed to create ttrpc server")
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
cmd/runhcs/container.go:493:			logrus.WithFields(logrus.Fields{
cmd/runhcs/container.go:524:	logrus.WithFields(logrus.Fields{
cmd/runhcs/vm.go:136:					logrus.WithError(err).
cmd/runhcs/vm.go:152:	logrus.WithFields(logrus.Fields{
hcn/hcnnamespace.go:360:	logrus.WithField("id", namespace.Id).Debugf("hcs::HostComputeNamespace::Sync")
hcn/hcnnamespace.go:392:		logrus.WithFields(f).
hcn/hcnnamespace.go:393:			WithError(err).
hcn/hcnsupport.go:81:		logrus.WithError(err).Errorf("unable to obtain supported features")
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
internal/cmd/cmd.go:228:				c.Log.WithError(err).Warn("failed to close Cmd stdin")
internal/cmd/cmd.go:237:				c.Log.WithError(cErr).Warn("failed to close Cmd stdout")
internal/cmd/cmd.go:247:				c.Log.WithError(cErr).Warn("failed to close Cmd stderr")
internal/cmd/cmd.go:278:		c.Log.WithError(waitErr).Warn("process wait failed")
internal/cmd/cmd.go:306:					c.Log.WithField("timeout", c.CopyAfterExitTimeout).Warn(err.Error())
internal/cmd/io.go:71:		log = log.WithFields(logrus.Fields{
internal/cmd/io.go:77:			log = log.WithError(err)
internal/cmd/io_binary.go:70:			log.G(ctx).WithError(err).Errorf("error closing wait pipe: %s", waitPipePath)
internal/cmd/io_binary.go:110:	log.G(ctx).WithFields(logrus.Fields{
internal/cmd/io_binary.go:186:				log.G(ctx).WithError(err).Errorf("error while closing stdout npipe")
internal/cmd/io_binary.go:192:				log.G(ctx).WithError(err).Errorf("error while closing stderr npipe")
internal/cmd/io_binary.go:205:				log.G(ctx).WithError(err).Errorf("error while waiting for binary cmd to finish")
internal/cmd/io_binary.go:211:				log.G(ctx).WithError(err).Errorf("error while killing binaryIO process")
internal/cmd/io_binary.go:293:		log.G(context.TODO()).WithError(err).Debug("error closing pipe listener")
internal/cmd/io_npipe.go:26:	log.G(ctx).WithFields(logrus.Fields{
internal/cmd/io_npipe.go:111:				log.G(nprw.ctx).WithFields(logrus.Fields{
internal/cmd/io_npipe.go:120:					log.G(nprw.ctx).WithField("address", nprw.pipePath).Info("Succeeded in reconnecting to named pipe")
internal/cmd/io_npipe.go:263:			logrus.WithError(err).Error("failed to accept pipe")
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
internal/controller/vm/vm_wcow.go:54:			logrus.WithError(err).Error("failed to listen for windows logging connections")
internal/controller/vm/vm_wcow.go:71:				logrus.WithError(err).Error("failed to connect to log socket")
internal/credentials/credentials.go:51:	log.G(ctx).WithField("containerID", id).Debug("creating container credential guard instance")
internal/credentials/credentials.go:118:	log.G(ctx).WithField("containerID", id).Debug("removing container credential guard")
internal/devices/assigned_devices.go:37:				log.G(ctx).WithError(releaseErr).Error("failed to release container resource")
internal/devices/assigned_devices.go:49:		log.G(ctx).WithField("vmbus id", vmBusInstanceID).Info("vmbus instance ID")
internal/devices/drivers.go:37:				log.G(ctx).WithError(releaseErr).Error("failed to release container resource")
internal/devices/pnp.go:60:		log.G(ctx).WithFields(logrus.Fields{
internal/devices/pnp.go:67:	log.G(ctx).WithField("added drivers", driverDir).Debug("installed drivers")
internal/gcs-sidecar/bridge.go:141:		logrus.WithFields(logrus.Fields{
internal/gcs-sidecar/bridge.go:189:		logrus.WithError(err).Errorf("error reading message header")
internal/gcs-sidecar/bridge.go:349:					log.G(ctx).WithError(err).Error("failed to send request to shimRequestChan")
internal/gcs-sidecar/bridge.go:374:					log.G(req.ctx).WithError(err).Errorf("failed to serve request: %v", req.header.Type.String())
internal/gcs-sidecar/bridge.go:386:						log.G(req.ctx).WithError(err).Errorf("failed to send response to shim")
internal/gcs-sidecar/bridge.go:455:						log.G(ctx).WithError(err).Error("failed to unmarshal the request")
internal/gcs-sidecar/bridge.go:473:					log.G(ctx).WithError(err).Error("failed to send request to b.sendToShimCh")
internal/gcs-sidecar/handlers.go:97:						log.G(ctx).WithField("name", value.Name).Trace("Registry value matches default, accepting without policy check")
internal/gcs-sidecar/handlers.go:114:					log.G(ctx).WithError(err).Warn("Registry changes validation failed - rejecting")
internal/gcs-sidecar/handlers.go:150:					log.G(ctx).WithError(removeErr).Errorf("Failed to remove container: %v", containerID)
internal/gcs-sidecar/handlers.go:729:							log.G(ctx).WithFields(map[string]interface{}{
internal/gcs-sidecar/host.go:108:	logrus.WithFields(logrus.Fields{
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
internal/gcs/bridge.go:230:		brdg.log.WithField("reason", ctx.Err()).Warn("ignoring response to bridge message")
internal/gcs/bridge.go:296:		brdg.log.WithFields(logrus.Fields{
internal/gcs/bridge.go:316:					brdg.log.WithFields(logrus.Fields{
internal/gcs/bridge.go:404:			brdg.log.WithError(err).Warning("could not scrub bridge payload")
internal/gcs/bridge.go:406:		brdg.log.WithFields(logrus.Fields{
internal/gcs/bridge.go:446:			brdg.log.WithError(err).Error("bridge write failed but call is already complete")
internal/gcs/container.go:202:			log.G(ctx).WithError(err).Warn("ignoring missing container")
internal/gcs/guestconnection.go:290:	logrus.WithField(logfields.ContainerID, cid).Info("container terminated in guest")
internal/gcs/guestconnection.go:323:					log.G(ctx).WithError(err).Warn("failed to encode OpenCensus Tracestate")
internal/gcs/process.go:110:	log.G(ctx).WithField("pid", p.id).Debug("created process pid")
internal/gcs/process.go:135:		log.G(ctx).WithError(err).Warn("close stdin failed")
internal/gcs/process.go:138:		log.G(ctx).WithError(err).Warn("close stdout failed")
internal/gcs/process.go:141:		log.G(ctx).WithError(err).Warn("close stderr failed")
internal/gcs/process.go:257:			log.G(ctx).WithFields(logrus.Fields{
internal/gcs/process.go:290:		log.G(ctx).WithError(err).Error("failed wait")
internal/gcs/process.go:292:	log.G(ctx).WithField("exitCode", ec).Debug("process exited")
internal/guest/bridge/bridge.go:83:		logrus.WithFields(logrus.Fields{
internal/guest/bridge/bridge.go:325:						entry.WithError(err).Warning("could not scrub bridge payload")
internal/guest/bridge/bridge.go:327:					entry.WithField("message", s).Trace("request read message")
internal/guest/bridge/bridge.go:391:				log.G(resp.ctx).WithField("message", string(responseBytes)).Trace("request write response")
internal/guest/bridge/bridge_v2.go:202:	log.G(ctx).WithField("pid", pid).Debug("created process pid")
internal/guest/kmsg/kmsg.go:111:		logrus.WithError(err).Error("failed to open /dev/kmsg")
internal/guest/kmsg/kmsg.go:129:			logrus.WithError(err).Error("kmsg read failure")
internal/guest/kmsg/kmsg.go:135:			logrus.WithFields(logrus.Fields{
internal/guest/kmsg/kmsg.go:141:				logrus.WithFields(entry.logFormat()).Info("kmsg read")
internal/guest/network/netns.go:102:	entry.WithField("namespace", ns).Debug("New network namespace from PID")
internal/guest/network/netns.go:114:		entry.WithField("mtu", mtu).Debug("EncapOverhead non-zero, will set MTU")
internal/guest/network/netns.go:134:		entry.WithField("timeout", timeout.String()).Debug("Execing udhcpc with timeout...")
internal/guest/network/netns.go:151:			entry.WithField("timeout", timeout.String()).Warningf("udhcpc timed out [%s]", cos)
internal/guest/network/netns.go:160:				entry.WithError(err).Debugf("udhcpc failed [%s]", cos)
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
internal/guest/runtime/hcsv2/container.go:241:			entity.WithError(err).Error("failed to unmount sandbox mounts")
internal/guest/runtime/hcsv2/container.go:246:			entity.WithError(err).Error("failed to unmount tmpfs sandbox mounts")
internal/guest/runtime/hcsv2/container.go:251:			entity.WithError(err).Error("failed to unmount hugepages mounts")
internal/guest/runtime/hcsv2/container.go:280:	log.G(ctx).WithField(logfields.ContainerID, c.id).Info("opengcs::Container::Update")
internal/guest/runtime/hcsv2/nvidia_utils.go:75:		log.G(ctx).WithField("hook", log.Format(ctx, nvidiaHook)).Debug("adding nvidia device runtime hook")
internal/guest/runtime/hcsv2/process.go:99:			log.G(ctx).WithError(err).Error("failed to wait for runc process")
internal/guest/runtime/hcsv2/process.go:102:		log.G(ctx).WithField("exitCode", p.exitCode).Debug("process exited")
internal/guest/runtime/hcsv2/process.go:113:				log.G(ctx).WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/process.go:244:		log.G(ctx).WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/uvm.go:161:		logrus.WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/uvm.go:335:		logrus.WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/uvm.go:365:		logrus.WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/uvm.go:469:					log.G(ctx).WithError(err).Debug("failed to add SEV device")
internal/guest/runtime/hcsv2/uvm.go:569:		log.G(ctx).WithFields(logrus.Fields{
internal/guest/runtime/hcsv2/uvm.go:660:		entry := log.G(ctx).WithField(logfields.Path, configFile)
internal/guest/runtime/hcsv2/uvm.go:663:			entry.WithError(err).Warning("could not scrub OCI spec written to config.json")
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
internal/guest/runtime/runc/process.go:67:			l.WithError(err).Error("failed to terminate container after process wait")
internal/guest/runtime/runc/process.go:89:	l.WithField(logfields.ProcessID, p.pid).Debug("relay wait completed")
internal/guest/spec/spec.go:52:	logrus.WithFields(logrus.Fields{
internal/guest/spec/spec.go:467:		log.G(ctx).WithField("sizeKB", val).Debug("set custom /dev/shm size")
internal/guest/spec/spec.go:475:			log.G(ctx).WithFields(logrus.Fields{
internal/guest/spec/spec.go:479:			}).WithError(err).Warning("annotation value could not be parsed")
internal/guest/spec/spec_devices.go:52:		entry := log.G(ctx).WithField("windows-device", log.Format(ctx, d))
internal/guest/spec/spec_devices.go:63:			entry.WithField("path", fullPCIPath).Trace("found PCI path for Windows device")
internal/guest/spec/spec_devices.go:71:				entry.WithFields(logrus.Fields{
internal/guest/spec/spec_devices.go:116:			entry := log.G(ctx).WithField("host-device", log.Format(ctx, d))
internal/guest/spec/spec_devices.go:137:				entry.WithError(err).Debugf("failed to find sysfs path for device %s", d.Path)
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
internal/guest/storage/crypt/crypt.go:59:						log.G(ctx).WithError(err).Warning("cryptsetup failed, context timeout")
internal/guest/storage/crypt/crypt.go:157:			log.G(ctx).WithError(err).Debugf("failed to delete temporary folder: %s", tempDir)
internal/guest/storage/crypt/crypt.go:180:				log.G(ctx).WithError(inErr).Debug("failed to cleanup crypt device")
internal/guest/storage/devicemapper/devicemapper.go:237:		log.G(ctx).WithError(err).Warning("CreateDevice error")
internal/guest/storage/devicemapper/devicemapper.go:248:					log.G(ctx).WithError(err).Error("CreateDeviceWithRetryErrors failed, context timeout")
internal/guest/storage/overlay/overlay.go:36:		log.G(ctx).WithError(statErr).WithField("path", filepath.Dir(path)).Warn("failed to get disk information for ENOSPC error")
internal/guest/storage/overlay/overlay.go:49:	log.G(ctx).WithFields(logrus.Fields{
internal/guest/storage/overlay/overlay.go:56:	}).WithError(err).Warn("got ENOSPC, gathering diagnostics")
internal/guest/storage/pmem/pmem.go:47:				log.G(ctx).WithError(err).Debugf("error cleaning up target: %s", target)
internal/guest/storage/pmem/pmem.go:104:					log.G(mCtx).WithError(err).Debugf("failed to cleanup linear target: %s", dmLinearName)
internal/guest/storage/pmem/pmem.go:118:					log.G(mCtx).WithError(err).Debugf("failed to cleanup verity target: %s", dmVerityName)
internal/guest/storage/pmem/pmem.go:151:			log.G(ctx).WithError(err).Debugf("failed to remove dm verity target: %s", dmVerityName)
internal/guest/storage/pmem/pmem.go:159:			log.G(ctx).WithError(err).Debugf("failed to remove dm linear target: %s", dmLinearName)
internal/guest/storage/scsi/scsi.go:163:						log.G(spnCtx).WithError(err).WithField("verityTarget", dmVerityName).Debug("failed to cleanup verity target")
internal/guest/storage/scsi/scsi.go:176:		log.G(ctx).WithFields(logrus.Fields{
internal/guest/storage/scsi/scsi.go:222:			log.G(ctx).WithError(err).Debug("get device filesystem failed, retrying in 500ms")
internal/guest/storage/scsi/scsi.go:230:		log.G(ctx).WithField("filesystem", deviceFS).Debug("filesystem found on device")
internal/guest/storage/scsi/scsi.go:302:		log.G(ctx).WithField("target", target).Trace("removing block device symlink")
internal/guest/storage/scsi/scsi.go:318:			log.G(ctx).WithError(err).Debugf("failed to remove dm verity target: %s", dmVerityName)
internal/guest/storage/scsi/scsi.go:365:					log.G(ctx).WithField("blockPath", blockPath).Warn(
internal/guest/storage/scsi/scsi.go:412:	log.G(ctx).WithField("devicePath", devicePath).Debug("found device path")
internal/guest/storage/scsi/scsi.go:432:			log.G(ctx).WithError(err).Warnf("failed to close file: %s", devicePath)
internal/guest/transport/devnull.go:23:	logrus.WithFields(logrus.Fields{
internal/guest/transport/log.go:19:	return &logConnection{c, logrus.WithField("port", port)}
internal/guest/transport/vsock.go:26:	logrus.WithFields(logrus.Fields{
internal/hcs/callback.go:149:	log := logrus.WithFields(logrus.Fields{
internal/hcs/errors.go:136:			log.G(ctx).WithError(err).Warning("Could not unmarshal HCS result")
internal/hcs/process.go:77:					log.G(ctx).WithError(err).Warn("force unblocking process waits")
internal/hcs/process.go:150:		log.G(ctx).WithField("err", err).Error("OpenComputeSystem() call failed")
internal/hcs/process.go:154:			log.G(ctx).WithField("err", err).Error("Terminate() call failed")
internal/hcs/process.go:230:		log.G(ctx).WithError(err).Error("failed wait")
internal/hcs/process.go:248:						log.G(ctx).WithField("wait-result", properties.LastWaitResult).Warning("non-zero last wait result")
internal/hcs/process.go:256:	log.G(ctx).WithField("exitCode", exitCode).Debug("process exited")
internal/hcs/system.go:414:					log.G(ctx).WithError(err).Warn("failed to get statistics in-proc")
internal/hcs/system.go:549:		logEntry = logEntry.WithError(fmt.Errorf("failed to query compute system properties in-proc: %w", err))
internal/hcs/system.go:553:	logEntry.WithFields(logrus.Fields{
internal/hcs/system.go:691:	log.G(ctx).WithField("pid", processInfo.ProcessId).Debug("created process pid")
internal/hcs/waithelper.go:37:		log.G(ctx).WithField("callbackNumber", callbackNumber).Error("failed to waitForNotification: callbackNumber does not exist in callbackMap")
internal/hcs/waithelper.go:45:		log.G(ctx).WithField("type", expectedNotification).Error("unknown notification type in waitForNotification")
internal/hcsoci/create.go:131:	log.G(ctx).WithFields(logrus.Fields{
internal/hcsoci/create.go:252:			log.G(ctx).WithError(err).Debug("failed to allocateLinuxResources")
internal/hcsoci/create.go:257:			log.G(ctx).WithError(err).Debug("failed createHCSContainerDocument")
internal/hcsoci/create.go:263:			log.G(ctx).WithError(err).Debug("failed to allocateWindowsResources")
internal/hcsoci/create.go:269:			log.G(ctx).WithError(err).Debug("failed createHCSContainerDocument")
internal/hcsoci/devices.go:110:					log.G(ctx).WithError(releaseErr).Error("failed to release container resource")
internal/hcsoci/devices.go:144:			log.G(ctx).WithField("parsed devices", specDev).Info("added windows device to spec")
internal/hcsoci/devices.go:167:					log.G(ctx).WithError(releaseErr).Error("failed to release container resource")
internal/hcsoci/devices.go:204:					log.G(ctx).WithError(releaseErr).Error("failed to release container resource")
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
internal/hvsocket/hvsocket.go:97:			log.G(context.Background()).WithError(closeErr).Debug("failed to close address info handle")
internal/jobcontainers/jobcontainer.go:102:	log.G(ctx).WithField("id", id).Debug("Creating job container")
internal/jobcontainers/jobcontainer.go:441:		log.G(context.Background()).WithError(err).WithField("cid", c.id).Warning("failed to close job object")
internal/jobcontainers/jobcontainer.go:446:		log.G(context.Background()).WithError(err).WithField("cid", c.id).Warning("failed to close token")
internal/jobcontainers/jobcontainer.go:453:			log.G(context.Background()).WithError(err).WithField("cid", c.id).Warning("failed to delete local account")
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
internal/jobcontainers/storage.go:54:					log.G(ctx).WithError(closeErr).Errorf("failed to cleanup mounted layers during another failure(%s)", err)
internal/jobobject/iocp.go:46:			log.G(ctx).WithError(err).Error("failed to poll for job object message")
internal/jobobject/iocp.go:52:				log.G(ctx).WithField("value", msq).Warn("encountered non queue type in job map")
internal/jobobject/iocp.go:57:				log.G(ctx).WithFields(logrus.Fields{
internal/jobobject/iocp.go:69:				log.G(ctx).WithFields(logrus.Fields{
internal/layers/lcow.go:51:		log.G(ctx).WithError(err).Error("failed RemoveCombinedLayersLCOW")
internal/layers/lcow.go:57:		log.G(ctx).WithError(err).Error("failed LCOW scratch mount release")
internal/layers/lcow.go:64:			log.G(ctx).WithFields(logrus.Fields{
internal/layers/lcow.go:97:	log.G(ctx).WithField("os", vm.OS()).Debug("hcsshim::MountLCOWLayers V2 UVM")
internal/layers/lcow.go:107:					log.G(ctx).WithError(err).Warn("failed to remove lcow layer on cleanup")
internal/layers/lcow.go:114:		log.G(ctx).WithField("layerPath", layer.VHDPath).Debug("mounting layer")
internal/layers/lcow.go:134:	log.G(ctx).WithField("hostPath", hostPath).Debug("mounting scratch VHD")
internal/layers/lcow.go:169:				log.G(ctx).WithError(err).Warn("failed to remove scratch on cleanup")
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
internal/layers/wcow_mount.go:507:				log.G(ctx).WithError(err).Warnf("mount process isolated forked CIM layers, undo failed with: %s", rErr)
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
internal/oci/annotations.go:51:					}).WithError(err).Warning("annotation expansion would overwrite conflicting value")
internal/oci/annotations.go:69:		log.G(ctx).WithFields(logrus.Fields{
internal/oci/annotations.go:74:		}).WithError(err).Warning("Host process container and disable host process container cannot both be true")
internal/oci/annotations.go:118:		entry := log.G(ctx).WithFields(logrus.Fields{
internal/oci/annotations.go:230:		entry := log.G(ctx).WithFields(logrus.Fields{
internal/oci/annotations.go:238:			entry.WithError(err).Warn("invalid GUID string for Hyper-V socket service configuration annotation")
internal/oci/annotations.go:250:			entry.WithFields(logrus.Fields{
internal/oci/annotations.go:256:			entry.WithField("configuration", log.Format(ctx, conf)).Trace("found Hyper-V socket service configuration annotation")
internal/oci/annotations.go:405:	entry := log.G(ctx).WithFields(logrus.Fields{
internal/oci/annotations.go:411:		entry = entry.WithError(err)
internal/oci/uvm.go:140:			log.G(ctx).WithFields(logrus.Fields{
internal/oci/uvm.go:294:			log.G(ctx).WithFields(logrus.Fields{
internal/resources/resources.go:137:					log.G(ctx).WithError(err).Error("failed to release container resource")
internal/resources/resources.go:144:				log.G(ctx).WithError(err).Error("failed to release container resource")
internal/resources/resources.go:153:					log.G(ctx).WithError(err).Error("failed to release container resource")
internal/schemaversion/schemaversion.go:98:			logrus.WithField("schemaVersion", requestedSV).Warn("Ignoring unsupported requested schema version")
internal/shim/publisher.go:109:			log.L.WithError(err).Error("forward event")
internal/shim/shim.go:198:	ctx = log.WithLogger(ctx, log.G(ctx).WithField("runtime", manager.Name()))
internal/shim/shim.go:261:		logger := log.G(ctx).WithFields(log.Fields{
internal/shim/shim.go:354:		log.G(ctx).WithFields(log.Fields{"id": pID, "type": p.Type}).Debug("loading plugin")
internal/shim/shim.go:388:				log.G(ctx).WithFields(log.Fields{"id": pID, "type": p.Type, "error": err}).Info("skip loading plugin")
internal/shim/shim.go:395:			log.G(ctx).WithField("id", pID).Debug("registering ttrpc service")
internal/shim/shim.go:466:			log.G(ctx).WithError(err).Fatal("containerd-shim: ttrpc server failure")
internal/shim/shim.go:481:			log.G(ctx).WithError(err).Warn("Could not setup pprof")
internal/shim/shim.go:485:	logger := log.G(ctx).WithFields(log.Fields{
internal/shim/shim.go:527:			log.G(ctx).WithError(err).Fatal("containerd-shim: pprof endpoint failure")
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
internal/tools/networkagent/main.go:468:		log.G(ctx).WithError(err).Fatalf("failed to read network agent's config file at %s", *configPath)
internal/tools/networkagent/main.go:470:	log.G(ctx).WithFields(logrus.Fields{
internal/tools/networkagent/main.go:488:		log.G(ctx).WithError(err).Fatalf("failed to connect to ncproxy at %s", conf.GRPCAddr)
internal/tools/networkagent/main.go:492:	log.G(ctx).WithField("addr", conf.GRPCAddr).Info("connected to ncproxy")
internal/tools/networkagent/main.go:510:		log.G(ctx).WithError(err).Fatalf("failed to listen on %s", grpcListener.Addr().String())
internal/tools/networkagent/main.go:523:	log.G(ctx).WithField("addr", conf.NodeNetSvcAddr).Info("serving network service agent")
internal/tools/networkagent/main.go:531:			log.G(ctx).WithError(err).Fatal("grpc service failure")
internal/tools/networkagent/v0_service_wrapper.go:53:	log.G(ctx).WithField("req", req).Info("ConfigureNetworking request")
internal/tools/rootfs/main.go:70:				logrus.WithFields(logrus.Fields{
internal/tools/rootfs/merge.go:141:	logrus.WithFields(logrus.Fields{
internal/tools/rootfs/merge.go:178:	logrus.WithFields(logrus.Fields{
internal/tools/rootfs/merge.go:204:		logrus.WithFields(logrus.Fields{
internal/tools/rootfs/merge.go:259:		entry := logrus.WithFields(logrus.Fields{
internal/tools/rootfs/merge.go:321:			entry.WithFields(logrus.Fields{
internal/tools/rootfs/merge.go:336:			entry.WithFields(logrus.Fields{
internal/tools/rootfs/merge.go:393:	entry := logrus.WithField("output", p)
internal/tools/rootfs/merge.go:403:		entry.WithError(err).Warn("unable to stat")
internal/tools/uvmboot/lcow.go:292:		entry := log.G(ctx).WithField("flag-value", s)
internal/tools/uvmboot/lcow.go:296:			entry.WithField(logrus.ErrorKey, "missing `=` in annotation").Warnf("invald %s flag value", name)
internal/tools/uvmboot/lcow.go:298:			entry.WithField(logrus.ErrorKey, "empty annotation key or value").Warnf("invald %s flag value", name)
internal/tools/uvmboot/lcow.go:300:			entry = entry.WithFields(logrus.Fields{
internal/tools/uvmboot/lcow.go:307:				entry.WithField(logfields.Value+"-existing", vv).Warn("overriding existing annotation")
internal/tools/uvmboot/lcow.go:364:			log.G(ctx).WithError(err).Warn("could not create console from stdin")
internal/tools/uvmboot/main.go:160:					logrus.WithField("uvm-id", id).WithError(err).Error("failed to run UVM")
internal/tools/uvmboot/mounts.go:34:		log.G(ctx).WithFields(logrus.Fields{
internal/tools/uvmboot/mounts.go:62:		log.G(ctx).WithFields(logrus.Fields{
internal/tools/uvmboot/mounts.go:99:		entry := log.G(ctx).WithField("flag-value", s)
internal/tools/uvmboot/mounts.go:102:			entry.WithError(err).Warnf("invald %s flag value", name)
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
internal/uvm/create.go:337:		e := log.G(ctx).WithField("VMGS file", vmgsFullPath)
internal/uvm/create.go:340:			e.WithError(err).Error("failed to remove VMGS file")
internal/uvm/create_lcow.go:159:		log.G(ctx).WithFields(logrus.Fields{
internal/uvm/create_lcow.go:176:		log.G(ctx).WithFields(logrus.Fields{
internal/uvm/create_lcow.go:190:			log.G(ctx).WithFields(logrus.Fields{
internal/uvm/create_lcow.go:542:		log.G(ctx).WithField("options", log.Format(ctx, opts)).Trace("makeLCOWDoc")
internal/uvm/create_lcow.go:630:		log.G(ctx).WithField("resource-partition-id", opts.ResourcePartitionID.String()).Debug("setting resource partition ID")
internal/uvm/create_lcow.go:740:							log.G(ctx).WithError(err).Debug("failed to release memory region")
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
internal/uvm/network.go:100:	l := log.G(ctx).WithField("netns-id", netNS)
internal/uvm/network.go:313:			log.G(ctx).WithFields(logrus.Fields{
internal/uvm/security_policy.go:60:				log.G(ctx).WithField("filePath", fullFilePath).Debug("UVM reference info file not found")
internal/uvm/start.go:78:	e := log.G(ctx).WithField(logfields.UVMID, uvm.id)
internal/uvm/start.go:92:				e.WithError(err).Error("failed to connect to entropy socket")
internal/uvm/start.go:98:				e.WithError(err).Error("failed to write entropy")
internal/uvm/start.go:121:						e.WithError(err).Error("failed to connect to log socket")
internal/uvm/start.go:143:					e.WithError(err).Error("failed to connect to log socket")
internal/uvm/start.go:293:			e.WithError(err).Error("failed to set log sources")
internal/uvm/start.go:296:			e.WithError(err).Error("failed to start log forwarding")
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
internal/wclayer/cim/mount.go:128:	log.L.WithFields(logrus.Fields{
internal/wclayer/layerutils.go:89:			logrus.WithError(err).Debug("Failed to convert name to guid")
internal/wclayer/layerutils.go:95:			logrus.WithError(err).Debug("Failed conversion of parentLayerPath to pointer")
internal/winapi/cimfs/cimfs.go:54:		logrus.WithField("path", windows.UTF16ToString(buf[:n])).Info("loaded cimfs.dll")
internal/winapi/cimwriter/cimwriter.go:49:			logrus.WithError(freeErr).Warn("failed to free cimwriter.dll after load failure")
internal/winapi/cimwriter/cimwriter.go:56:		logrus.WithField("path", windows.UTF16ToString(buf[:n])).Info("loaded cimwriter.dll")
internal/windevice/devicequery.go:238:	log.G(ctx).WithFields(logrus.Fields{
internal/windevice/devicequery.go:253:	log.G(ctx).WithField("interface list size", interfaceListSize).Trace("retrieved device interface list size")
internal/windevice/devicequery.go:293:		log.G(ctx).WithFields(logrus.Fields{
pkg/cimfs/cim_writer_windows.go:365:		log.G(ctx).WithError(err).Warnf("get region files for cim %s", cimPath)
pkg/cimfs/cim_writer_windows.go:372:		log.G(ctx).WithError(err).Warnf("get objectid file for cim %s", cimPath)
pkg/cimfs/cim_writer_windows.go:378:	log.G(ctx).WithFields(logrus.Fields{
pkg/cimfs/cim_writer_windows.go:386:			log.G(ctx).WithError(err).Warnf("remove file %s", regFilePath)
pkg/cimfs/cim_writer_windows.go:395:			log.G(ctx).WithError(err).Warnf("remove file %s", objFilePath)
pkg/cimfs/cim_writer_windows.go:403:		log.G(ctx).WithError(err).Warnf("remove file %s", cimPath)
pkg/cimfs/cimfs.go:19:		logrus.WithError(err).Warn("get build revision")
pkg/cimfs/common.go:108:			log.G(ctx).WithError(err).Warnf("stat for object file %s", path)
pkg/cimfs/common.go:134:			log.G(ctx).WithError(err).Warnf("stat for region file %s", path)
pkg/ociwclayer/cim/import.go:38:	log.G(ctx).WithFields(logrus.Fields{
pkg/ociwclayer/cim/import.go:129:	log.G(ctx).WithField("layer", layer).Debug("Importing block CIM layer from tar")
pkg/ociwclayer/cim/import.go:143:	log.G(ctx).WithField("config", *config).Debug("layer import config")
pkg/ociwclayer/cim/import.go:284:				log.G(ctx).WithError(flushErr).Warn("flush buffer during layer write failed")
pkg/ociwclayer/cim/import.go:303:	log.G(ctx).WithFields(logrus.Fields{
pkg/ociwclayer/cim/import.go:335:				log.G(ctx).WithError(retErr).Warnf("error in cleanup on failure: %s", rmErr)
pkg/securitypolicy/securitypolicy_options.go:113:	log.G(ctx).WithField("fragment", fmt.Sprintf("%+v", fragment)).Debug("VerifyAndExtractFragment")
pkg/securitypolicy/securitypolicy_options.go:139:	log.G(ctx).WithFields(logrus.Fields{
pkg/securitypolicy/securitypolicy_options.go:146:	log.G(ctx).WithFields(logrus.Fields{
pkg/securitypolicy/securitypolicyenforcer_rego.go:270:		return nil, policy.denyWithError(ctx, err, input)
pkg/securitypolicy/securitypolicyenforcer_rego.go:275:		return result, policy.denyWithError(ctx, err, input)
pkg/securitypolicy/securitypolicyenforcer_rego.go:280:		return nil, policy.denyWithError(ctx, err, input)
pkg/securitypolicy/securitypolicyenforcer_rego.go:322:		log.G(ctx).WithError(err).Error("unable to marshal error object")
pkg/securitypolicy/securitypolicyenforcer_rego.go:326:	log.G(ctx).WithField("policyDecision", string(decisionJSON))
pkg/securitypolicy/securitypolicyenforcer_rego.go:346:			log.G(ctx).WithError(err).Error("unable to marshal error object")
pkg/securitypolicy/securitypolicyenforcer_rego.go:360:func (policy *regoEnforcer) denyWithError(ctx context.Context, policyError error, input inputData) error {
pkg/securitypolicy/securitypolicyenforcer_rego.go:390:		log.G(ctx).WithError(err).Warn("unable to obtain reason for policy decision")
pkg/securitypolicy/securitypolicyenforcer_rego.go:763:			log.G(ctx).WithError(err).Warn("failed to obtain policy metadata snapshot")
test/functional/main_test.go:182:		log.G(ctx).WithField("features", flagFeatures.String()).Debug("provided features")
test/functional/main_test.go:201:			logrus.WithField("image", wcow.nanoserver.Image).Info("using Nano Server image")
test/functional/main_test.go:202:			logrus.WithField("image", wcow.servercore.Image).Info("using Server Core image")
test/functional/main_test.go:220:					log.G(ctx).WithFields(logrus.Fields{
test/functional/main_test.go:245:					logrus.WithError(err).Error("could not create ETW logrus hook")
test/functional/main_test.go:368:	e := log.G(ctx).WithFields(logrus.Fields{
test/functional/main_test.go:377:		e.WithFields(logrus.Fields{
test/functional/main_test.go:382:		e.WithField(
test/gcs/main_test.go:66:		logrus.WithError(err).Fatal("could not set up testing")
test/internal/layers/lazy.go:62:	log.G(ctx).WithFields(logrus.Fields{
test/internal/layers/lazy.go:124:	log.G(ctx).WithFields(logrus.Fields{
test/internal/layers/lazy.go:248:	log.L.WithField("privileges", privs).Infof("enableing process privileges")
test/internal/layers/lazy.go:288:		log.L.WithFields(logrus.Fields{
test/internal/layers/lazy.go:292:		}).WithError(err).Warning("failed to add MS defender exclusion for image layers directory")
test/internal/layers/lazy.go:294:		log.L.WithField("path", dir).Info("added MS Defender exclusion for image layers directory")
vendor/github.com/Microsoft/cosesign1go/pkg/cosesign1/create.go:57:		logrus.WithField("key", signingKey).Debug("parsed EC signing (private) key")
vendor/github.com/Microsoft/cosesign1go/pkg/cosesign1/create.go:63:			logrus.WithField("key", signingKey).Debug("parsed PKCS8 signing (private) key")
vendor/github.com/Microsoft/cosesign1go/pkg/cosesign1/create.go:70:			logrus.WithField("key", signingKey).Debug("parsed PKCS1 signing (private) key")
vendor/github.com/Microsoft/cosesign1go/pkg/cosesign1/create.go:75:		logrus.WithError(err).Debug("failed to decode a key")
vendor/github.com/Microsoft/cosesign1go/pkg/cosesign1/create.go:88:		logrus.WithField("leaf cert", fmt.Sprintf("%v", *chainCerts[0])).Debug("parsed cert chain for leaf")
vendor/github.com/Microsoft/cosesign1go/pkg/cosesign1/create.go:90:		logrus.WithError(err).Debug("cert parsing failed")
vendor/github.com/Microsoft/cosesign1go/pkg/cosesign1/create.go:114:		logrus.WithError(err).Error("failed to initialize cose signer")
vendor/github.com/Microsoft/cosesign1go/pkg/cosesign1/create.go:138:		logrus.WithError(err).Debug("failed to create cose.Sign1")
vendor/github.com/Microsoft/go-winio/pkg/etw/fieldopt.go:19:// WithFields returns the variadic arguments as a single slice.
vendor/github.com/Microsoft/go-winio/pkg/etw/fieldopt.go:20:func WithFields(opts ...FieldOpt) []FieldOpt {
vendor/github.com/containerd/containerd/v2/core/mount/mount_linux.go:258:				log.L.WithError(err).Warnf("failed to unmount temp lowerdir %s", lowerDir)
vendor/github.com/containerd/containerd/v2/core/mount/mount_linux.go:264:				log.L.WithError(err).Warnf("failed to remove temporary overlay lowerdir")
vendor/github.com/containerd/containerd/v2/core/mount/mount_linux.go:270:			log.L.WithError(err).Infof("failed to remove temporary overlay dir")
vendor/github.com/containerd/containerd/v2/core/mount/mount_windows.go:59:				log.G(context.TODO()).WithError(layerErr).Error("failed to deactivate layer during mount failure cleanup")
vendor/github.com/containerd/containerd/v2/core/mount/mount_windows.go:71:				log.G(context.TODO()).WithError(layerErr).Error("failed to unprepare layer during mount failure cleanup")
vendor/github.com/containerd/containerd/v2/core/mount/mount_windows.go:95:				log.G(context.TODO()).WithError(bindErr).Error("failed to remove binding during mount failure cleanup")
vendor/github.com/containerd/containerd/v2/core/mount/temp.go:50:			log.G(ctx).WithError(uerr).WithField("dir", root).Error("failed to remove mount temp dir")
vendor/github.com/containerd/containerd/v2/pkg/shim/publisher.go:106:			log.L.WithError(err).Error("forward event")
vendor/github.com/containerd/containerd/v2/pkg/shim/shim.go:196:	ctx = log.WithLogger(ctx, log.G(ctx).WithField("runtime", manager.Name()))
vendor/github.com/containerd/containerd/v2/pkg/shim/shim.go:259:		logger := log.G(ctx).WithFields(log.Fields{
vendor/github.com/containerd/containerd/v2/pkg/shim/shim.go:350:		log.G(ctx).WithFields(log.Fields{"id": pID, "type": p.Type}).Debug("loading plugin")
vendor/github.com/containerd/containerd/v2/pkg/shim/shim.go:384:				log.G(ctx).WithFields(log.Fields{"id": pID, "type": p.Type, "error": err}).Info("skip loading plugin")
vendor/github.com/containerd/containerd/v2/pkg/shim/shim.go:391:			log.G(ctx).WithField("id", pID).Debug("registering ttrpc service")
vendor/github.com/containerd/containerd/v2/pkg/shim/shim.go:459:			log.G(ctx).WithError(err).Fatal("containerd-shim: ttrpc server failure")
vendor/github.com/containerd/containerd/v2/pkg/shim/shim.go:465:			log.G(ctx).WithError(err).Warn("Could not setup pprof")
vendor/github.com/containerd/containerd/v2/pkg/shim/shim.go:469:	logger := log.G(ctx).WithFields(log.Fields{
vendor/github.com/containerd/containerd/v2/pkg/shim/shim.go:511:			log.G(ctx).WithError(err).Fatal("containerd-shim: pprof endpoint failure")
vendor/github.com/containerd/containerd/v2/pkg/shim/shim_unix.go:69:	log.L.WithField("socket", path).Debug("serving api on socket")
vendor/github.com/containerd/containerd/v2/pkg/shim/shim_unix.go:86:					logger.WithError(err).Error("reap exit status")
vendor/github.com/containerd/containerd/v2/pkg/shim/shim_unix.go:101:			logger.WithField("signal", s).Debugf("Caught exit signal")
vendor/github.com/containerd/containerd/v2/pkg/sys/pidfd_linux.go:41:			logger.WithError(err).Error("failed to ensure the kernel supports pidfd")
vendor/github.com/containerd/go-runc/io_unix.go:56:				logrus.WithError(err).Debug("failed to chown stdin, ignored")
vendor/github.com/containerd/go-runc/io_unix.go:71:				logrus.WithError(err).Debug("failed to chown stdout, ignored")
vendor/github.com/containerd/go-runc/io_unix.go:86:				logrus.WithError(err).Debug("failed to chown stderr, ignored")
vendor/github.com/containerd/log/context.go:62:// Fields type to pass to "WithFields".
vendor/github.com/containerd/log/context.go:66:// [Entry.WithFields]. It's finally logged when Trace, Debug, Info, Warn,
vendor/github.com/containerd/log/context.go:170:// combination with logger.WithField(s) for great effect.
vendor/github.com/containerd/stargz-snapshotter/estargz/build.go:258:			pw.CloseWithError(err)
vendor/github.com/containerd/stargz-snapshotter/estargz/build.go:263:			pw.CloseWithError(err)
vendor/github.com/containerd/stargz-snapshotter/estargz/build.go:411:				pw.CloseWithError(fmt.Errorf("Failed to write tar header: %v", err))
vendor/github.com/containerd/stargz-snapshotter/estargz/build.go:415:				pw.CloseWithError(fmt.Errorf("Failed to write tar payload: %v", err))
vendor/github.com/containerd/ttrpc/client.go:371:				log.G(c.ctx).WithField("stream", sid).Error("ttrpc: received message on inactive stream")
vendor/github.com/containerd/ttrpc/client.go:376:				s.closeWithError(err)
vendor/github.com/containerd/ttrpc/client.go:379:					log.G(c.ctx).WithFields(log.Fields{"error": err, "stream": sid}).Error("ttrpc: failed to handle message")
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
vendor/github.com/sirupsen/logrus/CHANGELOG.md:199:* performance: avoid re-allocations on `WithFields` (#335)
vendor/github.com/sirupsen/logrus/CHANGELOG.md:210:* logrus/core: support `WithError` on logger
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
vendor/github.com/sirupsen/logrus/entry.go:37:// Defines the key when adding errors using WithError.
vendor/github.com/sirupsen/logrus/entry.go:41:// the fields passed with WithField{,s}. It's finally logged when Trace, Debug,
vendor/github.com/sirupsen/logrus/entry.go:106:func (entry *Entry) WithError(err error) *Entry {
vendor/github.com/sirupsen/logrus/entry.go:107:	return entry.WithField(ErrorKey, err)
vendor/github.com/sirupsen/logrus/entry.go:120:func (entry *Entry) WithField(key string, value interface{}) *Entry {
vendor/github.com/sirupsen/logrus/entry.go:121:	return entry.WithFields(Fields{key: value})
vendor/github.com/sirupsen/logrus/entry.go:125:func (entry *Entry) WithFields(fields Fields) *Entry {
vendor/github.com/sirupsen/logrus/exported.go:54:// WithError creates an entry from the standard logger and adds an error to it, using the value defined in ErrorKey as key.
vendor/github.com/sirupsen/logrus/exported.go:55:func WithError(err error) *Entry {
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
vendor/github.com/sirupsen/logrus/logger.go:130:// `WithError` for the given `error`.
vendor/github.com/sirupsen/logrus/logger.go:131:func (logger *Logger) WithError(err error) *Entry {
vendor/github.com/sirupsen/logrus/logger.go:134:	return entry.WithError(err)
vendor/github.com/sirupsen/logrus/logrus.go:9:// Fields type, used to pass to `WithFields`.
vendor/github.com/sirupsen/logrus/logrus.go:140:	WithField(key string, value interface{}) *Entry
vendor/github.com/sirupsen/logrus/logrus.go:141:	WithFields(fields Fields) *Entry
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

# logrus.Entry Usage

Severity: **critical**

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

# Formatter / Hook Usage

Severity: **high**

```txt
cmd/containerd-shim-lcow-v2/main.go:29:	logrus.AddHook(log.NewHook())
cmd/containerd-shim-lcow-v2/main.go:35:	logrus.SetFormatter(log.NopFormatter{})
cmd/containerd-shim-lcow-v2/main.go:36:	logrus.SetOutput(io.Discard)
cmd/containerd-shim-lcow-v2/main.go:83:				logrus.SetLevel(lvl)
cmd/containerd-shim-lcow-v2/manager.go:121:	logrus.SetOutput(io.Discard)
cmd/containerd-shim-lcow-v2/service/plugin/plugin.go:40:			logrus.AddHook(hook)
cmd/containerd-shim-runhcs-v1/main.go:72:	logrus.AddHook(log.NewHook())
cmd/containerd-shim-runhcs-v1/main.go:81:			logrus.AddHook(hook)
cmd/containerd-shim-runhcs-v1/serve.go:96:			logrus.SetLevel(logrus.DebugLevel)
cmd/containerd-shim-runhcs-v1/serve.go:106:			logrus.SetLevel(lvl)
cmd/containerd-shim-runhcs-v1/serve.go:111:			logrus.SetFormatter(&logrus.TextFormatter{
cmd/containerd-shim-runhcs-v1/serve.go:150:					logrus.SetOutput(cur)
cmd/containerd-shim-runhcs-v1/serve.go:158:			logrus.SetFormatter(hcslog.NopFormatter{})
cmd/containerd-shim-runhcs-v1/serve.go:159:			logrus.SetOutput(io.Discard)
cmd/containerd-shim-runhcs-v1/start.go:40:		logrus.SetOutput(io.Discard)
cmd/gcs-sidecar/main.go:149:	logrus.AddHook(shimlog.NewHook())
cmd/gcs-sidecar/main.go:155:	logrus.SetLevel(level)
cmd/gcs/main.go:225:	logrus.AddHook(log.NewHook())
cmd/gcs/main.go:247:		logrus.SetOutput(logWriter)
cmd/gcs/main.go:250:		logrus.SetOutput(io.Discard)
cmd/gcs/main.go:261:		logrus.SetFormatter(&logrus.JSONFormatter{
cmd/gcs/main.go:275:	logrus.SetLevel(level)
cmd/gcstools/commoncli/common.go:30:	logrus.SetLevel(level)
cmd/gcstools/commoncli/common.go:38:	logrus.SetOutput(outputTarget)
cmd/gcstools/installdrivers.go:90:	log.G(ctx).Logger.SetOutput(os.Stderr)
cmd/ncproxy/run.go:153:			logrus.AddHook(hook)
cmd/ncproxy/run.go:202:		logrus.SetOutput(io.Discard)
cmd/ncproxy/service.go:90:	log.SetOutput(os.Stderr)
cmd/runhcs/main.go:60:		logrus.AddHook(hook)
cmd/runhcs/main.go:135:			logrus.SetLevel(logrus.DebugLevel)
cmd/runhcs/main.go:148:			logrus.SetOutput(f)
cmd/runhcs/main.go:154:			logrus.SetFormatter(new(logrus.JSONFormatter))
cmd/runhcs/shim.go:52:			logrus.SetOutput(lpc)
cmd/runhcs/shim.go:54:			logrus.SetOutput(os.Stderr)
cmd/runhcs/vm.go:45:			logrus.SetOutput(lpc)
cmd/runhcs/vm.go:47:			logrus.SetOutput(os.Stderr)
internal/guest/bridge/bridge_unit_test.go:460:	logrus.SetOutput(io.Discard)
internal/guest/bridge/bridge_unit_test.go:514:	logrus.SetOutput(io.Discard)
internal/guest/bridge/bridge_unit_test.go:595:	logrus.SetOutput(io.Discard)
internal/oci/annotations_test.go:25:		logrus.SetLevel(l)
internal/oci/annotations_test.go:27:	logrus.SetLevel(logrus.ErrorLevel)
internal/oci/annotations_test.go:144:		logrus.SetLevel(l)
internal/oci/annotations_test.go:146:	logrus.SetLevel(logrus.ErrorLevel)
internal/shim/shim.go:181:		_ = log.SetLevel("debug")
internal/shim/shim.go:187:	l.Logger.SetOutput(f)
internal/shim/shim.go:266:			logger.Logger.SetLevel(log.DebugLevel)
internal/tools/rootfs/main.go:25:	logrus.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
internal/tools/rootfs/main.go:57:						logrus.SetLevel(lvl)
internal/tools/uvmboot/main.go:108:		logrus.SetLevel(lvl)
internal/vm/vmutils/gcs_logs_test.go:190:	logrus.SetOutput(io.Discard)
internal/vm/vmutils/gcs_logs_test.go:191:	logrus.AddHook(hook)
internal/vm/vmutils/gcs_logs_test.go:193:		logrus.SetOutput(originalOutput)
internal/vm/vmutils/gcs_logs_test.go:277:	logrus.SetOutput(io.Discard)
internal/vm/vmutils/gcs_logs_test.go:278:	logrus.AddHook(hook)
internal/vm/vmutils/gcs_logs_test.go:280:		logrus.SetOutput(originalOutput)
internal/vm/vmutils/gcs_logs_test.go:299:	logrus.SetOutput(io.Discard)
internal/vm/vmutils/gcs_logs_test.go:300:	logrus.AddHook(hook)
internal/vm/vmutils/gcs_logs_test.go:302:		logrus.SetOutput(originalOutput)
internal/vm/vmutils/gcs_logs_test.go:351:	logrus.SetOutput(io.Discard)
internal/vm/vmutils/gcs_logs_test.go:352:	logrus.AddHook(hook)
internal/vm/vmutils/gcs_logs_test.go:354:		logrus.SetOutput(originalOutput)
internal/vm/vmutils/gcs_logs_test.go:424:	logrus.SetOutput(io.Discard)
pkg/securitypolicy/securitypolicy_options.go:90:		logrus.SetOutput(s.logWriter)
pkg/securitypolicy/securitypolicy_options.go:92:		logrus.SetOutput(io.Discard)
test/functional/main_test.go:174:	logrus.SetOutput(os.Stdout)
test/functional/main_test.go:175:	logrus.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
test/functional/main_test.go:176:	logrus.SetLevel(flagLogLevel.Level)
test/functional/main_test.go:243:					logrus.AddHook(hook)
test/functional/main_test.go:250:			logrus.SetFormatter(log.NopFormatter{})
test/functional/main_test.go:251:			logrus.SetOutput(io.Discard)
test/functional/main_test.go:255:				logrus.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
test/functional/main_test.go:256:				logrus.SetOutput(os.Stdout)
test/gcs/main_test.go:115:	logrus.SetLevel(flagLogLevel.Level)
test/gcs/main_test.go:117:	logrus.SetOutput(os.Stdout)
test/gcs/main_test.go:118:	logrus.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
test/internal/schemaversion_test.go:18:	logrus.SetOutput(io.Discard)
vendor/github.com/containerd/containerd/v2/pkg/shim/shim.go:179:		_ = log.SetLevel("debug")
vendor/github.com/containerd/containerd/v2/pkg/shim/shim.go:185:	l.Logger.SetOutput(f)
vendor/github.com/containerd/containerd/v2/pkg/shim/shim.go:264:			logger.Logger.SetLevel(log.DebugLevel)
vendor/github.com/containerd/log/context.go:111:// SetLevel sets log level globally. It returns an error if the given
vendor/github.com/containerd/log/context.go:123:func SetLevel(level string) error {
vendor/github.com/containerd/log/context.go:129:	L.Logger.SetLevel(lvl)
vendor/github.com/containerd/log/context.go:154:		L.Logger.SetFormatter(&logrus.TextFormatter{
vendor/github.com/containerd/log/context.go:160:		L.Logger.SetFormatter(&logrus.JSONFormatter{
vendor/github.com/open-policy-agent/opa/logging/logging.go:35:	SetLevel(Level)
vendor/github.com/open-policy-agent/opa/logging/logging.go:60:// SetOutput sets the underlying logrus output.
vendor/github.com/open-policy-agent/opa/logging/logging.go:61:func (l *StandardLogger) SetOutput(w io.Writer) {
vendor/github.com/open-policy-agent/opa/logging/logging.go:62:	l.logger.SetOutput(w)
vendor/github.com/open-policy-agent/opa/logging/logging.go:65:// SetFormatter sets the underlying logrus formatter.
vendor/github.com/open-policy-agent/opa/logging/logging.go:66:func (l *StandardLogger) SetFormatter(formatter logrus.Formatter) {
vendor/github.com/open-policy-agent/opa/logging/logging.go:67:	l.logger.SetFormatter(formatter)
vendor/github.com/open-policy-agent/opa/logging/logging.go:88:// SetLevel sets the standard logger level.
vendor/github.com/open-policy-agent/opa/logging/logging.go:89:func (l *StandardLogger) SetLevel(level Level) {
vendor/github.com/open-policy-agent/opa/logging/logging.go:105:	l.logger.SetLevel(logrusLevel)
vendor/github.com/open-policy-agent/opa/logging/logging.go:197:// SetLevel set log level
vendor/github.com/open-policy-agent/opa/logging/logging.go:198:func (l *NoOpLogger) SetLevel(level Level) {
vendor/github.com/opencontainers/runc/libcontainer/init_linux.go:169:		logrus.SetLevel(logrus.Level(logLevel))
vendor/github.com/opencontainers/runc/libcontainer/init_linux.go:178:	logrus.SetOutput(logPipe)
vendor/github.com/opencontainers/runc/libcontainer/init_linux.go:179:	logrus.SetFormatter(new(logrus.JSONFormatter))
vendor/github.com/sirupsen/logrus/CHANGELOG.md:116:    * SetFormatter
vendor/github.com/sirupsen/logrus/CHANGELOG.md:117:    * SetOutput
vendor/github.com/sirupsen/logrus/CHANGELOG.md:133:  * a new SetOutput method in the Logger
vendor/github.com/sirupsen/logrus/CHANGELOG.md:154:* Make (*Logger) SetLevel a public method
vendor/github.com/sirupsen/logrus/README.md:43:With `log.SetFormatter(&log.JSONFormatter{})`, for easy parsing by logstash
vendor/github.com/sirupsen/logrus/README.md:63:With the default `log.SetFormatter(&log.TextFormatter{})` when a TTY is not
vendor/github.com/sirupsen/logrus/README.md:78:	log.SetFormatter(&log.TextFormatter{
vendor/github.com/sirupsen/logrus/README.md:147:  log.SetFormatter(&log.JSONFormatter{})
vendor/github.com/sirupsen/logrus/README.md:151:  log.SetOutput(os.Stdout)
vendor/github.com/sirupsen/logrus/README.md:154:  log.SetLevel(log.WarnLevel)
vendor/github.com/sirupsen/logrus/README.md:278:  log.AddHook(airbrake.NewHook(123, "xyz", "production"))
vendor/github.com/sirupsen/logrus/README.md:284:    log.AddHook(hook)
vendor/github.com/sirupsen/logrus/README.md:314:log.SetLevel(log.InfoLevel)
vendor/github.com/sirupsen/logrus/README.md:320:Note: If you want different log levels for global (`log.SetLevel(...)`) and syslog logging, please check the [syslog hook README](hooks/syslog/README.md#different-log-levels-for-local-and-remote-logging).
vendor/github.com/sirupsen/logrus/README.md:350:    log.SetFormatter(&log.JSONFormatter{})
vendor/github.com/sirupsen/logrus/README.md:353:    log.SetFormatter(&log.TextFormatter{})
vendor/github.com/sirupsen/logrus/README.md:399:log.SetFormatter(new(MyJSONFormatter))
vendor/github.com/sirupsen/logrus/README.md:440:log.SetOutput(logger.Writer())
vendor/github.com/sirupsen/logrus/exported.go:18:// SetOutput sets the standard logger output.
vendor/github.com/sirupsen/logrus/exported.go:19:func SetOutput(out io.Writer) {
vendor/github.com/sirupsen/logrus/exported.go:20:	std.SetOutput(out)
vendor/github.com/sirupsen/logrus/exported.go:23:// SetFormatter sets the standard logger formatter.
vendor/github.com/sirupsen/logrus/exported.go:24:func SetFormatter(formatter Formatter) {
vendor/github.com/sirupsen/logrus/exported.go:25:	std.SetFormatter(formatter)
vendor/github.com/sirupsen/logrus/exported.go:34:// SetLevel sets the standard logger level.
vendor/github.com/sirupsen/logrus/exported.go:35:func SetLevel(level Level) {
vendor/github.com/sirupsen/logrus/exported.go:36:	std.SetLevel(level)
vendor/github.com/sirupsen/logrus/exported.go:49:// AddHook adds a hook to the standard logger hooks.
vendor/github.com/sirupsen/logrus/exported.go:50:func AddHook(hook Hook) {
vendor/github.com/sirupsen/logrus/exported.go:51:	std.AddHook(hook)
vendor/github.com/sirupsen/logrus/logger.go:361:// SetLevel sets the logger level.
vendor/github.com/sirupsen/logrus/logger.go:362:func (logger *Logger) SetLevel(level Level) {
vendor/github.com/sirupsen/logrus/logger.go:371:// AddHook adds a hook to the logger hooks.
vendor/github.com/sirupsen/logrus/logger.go:372:func (logger *Logger) AddHook(hook Hook) {
vendor/github.com/sirupsen/logrus/logger.go:383:// SetFormatter sets the logger formatter.
vendor/github.com/sirupsen/logrus/logger.go:384:func (logger *Logger) SetFormatter(formatter Formatter) {
vendor/github.com/sirupsen/logrus/logger.go:390:// SetOutput sets the logger output.
vendor/github.com/sirupsen/logrus/logger.go:391:func (logger *Logger) SetOutput(output io.Writer) {
vendor/github.com/urfave/cli/flag.go:122:	set.SetOutput(ioutil.Discard)
vendor/github.com/urfave/cli/v2/flag.go:181:	set.SetOutput(io.Discard)
vendor/go.opentelemetry.io/otel/semconv/v1.39.0/attribute_group.go:10346:	McpMethodNameLoggingSetLevel = McpMethodNameKey.String("logging/setLevel")
vendor/golang.org/x/tools/internal/stdlib/manifest.go:5031:		{"(*FlagSet).SetOutput", Method, 0, ""},
vendor/golang.org/x/tools/internal/stdlib/manifest.go:7232:		{"(*Logger).SetOutput", Method, 5, ""},
vendor/golang.org/x/tools/internal/stdlib/manifest.go:7259:		{"SetOutput", Func, 0, "func(w io.Writer)"},
```

# Context Logger Usage

Severity: **important**

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

# Public API Exposure

Severity: **critical**

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

# Package Risk Ranking

| Package | Risk | Score |
|---|---|---|
| github.com/Microsoft/hcsshim | low | 0 |
| github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options | low | 0 |
| github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/stats | low | 0 |
| github.com/Microsoft/hcsshim/cmd/gcs | low | 0 |
| github.com/Microsoft/hcsshim/cmd/gcstools | low | 0 |
| github.com/Microsoft/hcsshim/cmd/gcstools/commoncli | low | 0 |
| github.com/Microsoft/hcsshim/cmd/gcstools/generichook | low | 0 |
| github.com/Microsoft/hcsshim/cmd/hooks/wait-paths | low | 0 |
| github.com/Microsoft/hcsshim/cmd/tar2ext4 | low | 0 |
| github.com/Microsoft/hcsshim/computestorage | low | 0 |
| github.com/Microsoft/hcsshim/ext4/dmverity | low | 0 |
| github.com/Microsoft/hcsshim/ext4/internal/compactext4 | low | 0 |
| github.com/Microsoft/hcsshim/ext4/internal/format | low | 0 |
| github.com/Microsoft/hcsshim/ext4/tar2ext4 | low | 0 |
| github.com/Microsoft/hcsshim/hcn | low | 0 |
| github.com/Microsoft/hcsshim/internal/annotations | low | 0 |
| github.com/Microsoft/hcsshim/internal/appargs | low | 0 |
| github.com/Microsoft/hcsshim/internal/bridgeutils/commonutils | low | 0 |
| github.com/Microsoft/hcsshim/internal/bridgeutils/gcserr | low | 0 |
| github.com/Microsoft/hcsshim/internal/builder/vm/lcow | low | 0 |
| github.com/Microsoft/hcsshim/internal/cmd | low | 0 |
| github.com/Microsoft/hcsshim/internal/cni | low | 0 |
| github.com/Microsoft/hcsshim/internal/computeagent | low | 0 |
| github.com/Microsoft/hcsshim/internal/computeagent/mock | low | 0 |
| github.com/Microsoft/hcsshim/internal/computecore | low | 0 |
| github.com/Microsoft/hcsshim/internal/conpty | low | 0 |
| github.com/Microsoft/hcsshim/internal/copyfile | low | 0 |
| github.com/Microsoft/hcsshim/internal/cpugroup | low | 0 |
| github.com/Microsoft/hcsshim/internal/credentials | low | 0 |
| github.com/Microsoft/hcsshim/internal/debug | low | 0 |
| github.com/Microsoft/hcsshim/internal/devices | low | 0 |
| github.com/Microsoft/hcsshim/internal/exec | low | 0 |
| github.com/Microsoft/hcsshim/internal/extendedtask | low | 0 |
| github.com/Microsoft/hcsshim/internal/gcs | low | 0 |
| github.com/Microsoft/hcsshim/internal/guest/bridge | low | 0 |
| github.com/Microsoft/hcsshim/internal/guest/kmsg | low | 0 |
| github.com/Microsoft/hcsshim/internal/guest/linux | low | 0 |
| github.com/Microsoft/hcsshim/internal/guest/network | low | 0 |
| github.com/Microsoft/hcsshim/internal/guest/prot | low | 0 |
| github.com/Microsoft/hcsshim/internal/guest/runtime | low | 0 |
| github.com/Microsoft/hcsshim/internal/guest/runtime/hcsv2 | low | 0 |
| github.com/Microsoft/hcsshim/internal/guest/runtime/runc | low | 0 |
| github.com/Microsoft/hcsshim/internal/guest/spec | low | 0 |
| github.com/Microsoft/hcsshim/internal/guest/stdio | low | 0 |
| github.com/Microsoft/hcsshim/internal/guest/storage | low | 0 |
| github.com/Microsoft/hcsshim/internal/guest/storage/crypt | low | 0 |
| github.com/Microsoft/hcsshim/internal/guest/storage/devicemapper | low | 0 |
| github.com/Microsoft/hcsshim/internal/guest/storage/ext4 | low | 0 |
| github.com/Microsoft/hcsshim/internal/guest/storage/overlay | low | 0 |
| github.com/Microsoft/hcsshim/internal/guest/storage/pci | low | 0 |

# Migration Recommendations

| Package | Risk | Suggested Strategy |
|---|---|---|
| github.com/Microsoft/hcsshim | low | backend swap only |
| github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options | low | backend swap only |
| github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/stats | low | backend swap only |
| github.com/Microsoft/hcsshim/cmd/gcs | low | backend swap only |
| github.com/Microsoft/hcsshim/cmd/gcstools | low | backend swap only |
| github.com/Microsoft/hcsshim/cmd/gcstools/commoncli | low | backend swap only |
| github.com/Microsoft/hcsshim/cmd/gcstools/generichook | low | backend swap only |
| github.com/Microsoft/hcsshim/cmd/hooks/wait-paths | low | backend swap only |
| github.com/Microsoft/hcsshim/cmd/tar2ext4 | low | backend swap only |
| github.com/Microsoft/hcsshim/computestorage | low | backend swap only |
| github.com/Microsoft/hcsshim/ext4/dmverity | low | backend swap only |
| github.com/Microsoft/hcsshim/ext4/internal/compactext4 | low | backend swap only |
| github.com/Microsoft/hcsshim/ext4/internal/format | low | backend swap only |
| github.com/Microsoft/hcsshim/ext4/tar2ext4 | low | backend swap only |
| github.com/Microsoft/hcsshim/hcn | low | backend swap only |
| github.com/Microsoft/hcsshim/internal/annotations | low | backend swap only |
| github.com/Microsoft/hcsshim/internal/appargs | low | backend swap only |
| github.com/Microsoft/hcsshim/internal/bridgeutils/commonutils | low | backend swap only |
| github.com/Microsoft/hcsshim/internal/bridgeutils/gcserr | low | backend swap only |
| github.com/Microsoft/hcsshim/internal/builder/vm/lcow | low | backend swap only |
| github.com/Microsoft/hcsshim/internal/cmd | low | backend swap only |
| github.com/Microsoft/hcsshim/internal/cni | low | backend swap only |
| github.com/Microsoft/hcsshim/internal/computeagent | low | backend swap only |
| github.com/Microsoft/hcsshim/internal/computeagent/mock | low | backend swap only |
| github.com/Microsoft/hcsshim/internal/computecore | low | backend swap only |
| github.com/Microsoft/hcsshim/internal/conpty | low | backend swap only |
| github.com/Microsoft/hcsshim/internal/copyfile | low | backend swap only |
| github.com/Microsoft/hcsshim/internal/cpugroup | low | backend swap only |
| github.com/Microsoft/hcsshim/internal/credentials | low | backend swap only |
| github.com/Microsoft/hcsshim/internal/debug | low | backend swap only |
| github.com/Microsoft/hcsshim/internal/devices | low | backend swap only |
| github.com/Microsoft/hcsshim/internal/exec | low | backend swap only |
| github.com/Microsoft/hcsshim/internal/extendedtask | low | backend swap only |
| github.com/Microsoft/hcsshim/internal/gcs | low | backend swap only |
| github.com/Microsoft/hcsshim/internal/guest/bridge | low | backend swap only |
| github.com/Microsoft/hcsshim/internal/guest/kmsg | low | backend swap only |
| github.com/Microsoft/hcsshim/internal/guest/linux | low | backend swap only |
| github.com/Microsoft/hcsshim/internal/guest/network | low | backend swap only |
| github.com/Microsoft/hcsshim/internal/guest/prot | low | backend swap only |
| github.com/Microsoft/hcsshim/internal/guest/runtime | low | backend swap only |
| github.com/Microsoft/hcsshim/internal/guest/runtime/hcsv2 | low | backend swap only |
| github.com/Microsoft/hcsshim/internal/guest/runtime/runc | low | backend swap only |
| github.com/Microsoft/hcsshim/internal/guest/spec | low | backend swap only |
| github.com/Microsoft/hcsshim/internal/guest/stdio | low | backend swap only |
| github.com/Microsoft/hcsshim/internal/guest/storage | low | backend swap only |
| github.com/Microsoft/hcsshim/internal/guest/storage/crypt | low | backend swap only |
| github.com/Microsoft/hcsshim/internal/guest/storage/devicemapper | low | backend swap only |
| github.com/Microsoft/hcsshim/internal/guest/storage/ext4 | low | backend swap only |
| github.com/Microsoft/hcsshim/internal/guest/storage/overlay | low | backend swap only |
| github.com/Microsoft/hcsshim/internal/guest/storage/pci | low | backend swap only |

# Critical Coupling Points

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

# Next Engineering Steps


1. Inspect internal/log and existing wrappers
2. Determine whether logrus is already partially abstracted
3. Identify logger bootstrap lifecycle
4. Prototype minimal logger interface
5. Implement noop logger
6. Implement logrus adapter
7. Benchmark before migration
8. Split rollout into multiple PRs

