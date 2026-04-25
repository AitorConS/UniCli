/*
 * QEMU fw_cfg interface — see drivers/fw_cfg.c for protocol details.
 */
#ifndef _FW_CFG_H_
#define _FW_CFG_H_

/* fw_cfg_present returns true if the QEMU fw_cfg device responds with the
 * "QEMU" signature on I/O ports 0x510/0x511. Cheap; safe to call on bare
 * metal — a missing device just returns false. */
boolean fw_cfg_present(void);

/* fw_cfg_read_file reads the contents of a named fw_cfg file (e.g.
 * "opt/uni/env") into a heap-allocated buffer. Returns INVALID_ADDRESS if
 * the device is absent, the file is missing, or allocation fails. */
buffer fw_cfg_read_file(heap h, sstring name);

#endif
