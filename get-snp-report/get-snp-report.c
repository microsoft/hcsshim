#include <sys/ioctl.h>
#include <sys/types.h>
#include <sys/stat.h>
#include <fcntl.h>
#include <stdio.h>
#include <memory.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <stdbool.h>

/* From sev-snp driver include/uapi/linux/psp-sev-guest.h */

struct sev_snp_guest_request {
	uint8_t req_msg_type;
	uint8_t rsp_msg_type;
	uint8_t msg_version;
	uint16_t request_len;
	uint64_t request_uaddr;
	uint16_t response_len;
	uint64_t response_uaddr;
	uint32_t error;		/* firmware error code on failure (see psp-sev.h) */
};

enum snp_msg_type {
	SNP_MSG_TYPE_INVALID = 0,
	SNP_MSG_CPUID_REQ,
	SNP_MSG_CPUID_RSP,
	SNP_MSG_KEY_REQ,
	SNP_MSG_KEY_RSP,
	SNP_MSG_REPORT_REQ,
	SNP_MSG_REPORT_RSP,
	SNP_MSG_EXPORT_REQ,
	SNP_MSG_EXPORT_RSP,
	SNP_MSG_IMPORT_REQ,
	SNP_MSG_IMPORT_RSP,
	SNP_MSG_ABSORB_REQ,
	SNP_MSG_ABSORB_RSP,
	SNP_MSG_VMRK_REQ,
	SNP_MSG_VMRK_RSP,
	SNP_MSG_TYPE_MAX
};

#define SEV_GUEST_IOC_TYPE		'S'
#define SEV_SNP_GUEST_MSG_REQUEST	_IOWR(SEV_GUEST_IOC_TYPE, 0x0, struct sev_snp_guest_request)
#define SEV_SNP_GUEST_MSG_REPORT	_IOWR(SEV_GUEST_IOC_TYPE, 0x1, struct sev_snp_guest_request)
#define SEV_SNP_GUEST_MSG_KEY		_IOWR(SEV_GUEST_IOC_TYPE, 0x2, struct sev_snp_guest_request)

#define PRINT_VAL(ptr, field) printBytes(#field, (const uint8_t *)&(ptr->field), sizeof(ptr->field), true)
#define PRINT_BYTES(ptr, field) printBytes(#field, (const uint8_t *)&(ptr->field), sizeof(ptr->field), false)

/* from SEV-SNP Firmware ABI Specification Table 20 */

typedef struct {
    uint8_t report_data[64];
    uint32_t vmpl;
    uint8_t reserved[28]; // needs to be zero
} msg_report_req;

/* from SEV-SNP Firmware ABI Specification from Table 21 */

typedef struct {
    uint32_t    version;            // version no. of this attestation report. Set to 1 for this specification. Now 2 with production silicon
    uint32_t    guest_svn;          // The guest SVN
    uint64_t    policy;             // see table 8 - various settings
    __uint128_t family_id;          // as provided at launch
    __uint128_t image_id;           // as provided at launch
    uint32_t    vmpl;               // the request VMPL for the attestation report
    uint32_t    signature_algo;
    uint64_t    platform_version;   // The install version of the firmware
    uint64_t    platform_info;      // information about the platform see table 22
    // not going to try to use bit fields for this next one. Too confusing as to which bit of the byte will be used. Make a mask if you need it
    uint32_t    author_key_en;      // 31 bits of reserved, must be zero, bottom bit indicates that the digest of the
                                    // author key is present in AUTHOR_KEY_DIGEST. Set to the value of GCTX.AuthorKeyEn.
    uint32_t    reserved1;          // must be zero
    uint8_t     report_data[64];    // Guest provided data.
    uint8_t     measurement[48];    // measurement calculated at launch
    uint8_t     host_data[32];      // data provided by the hypervisor at launch
    uint8_t     id_key_digest[48];  // SHA-384 digest of the ID public key that signed the ID block provided in SNP_LAUNCH_FINISH
    uint8_t     author_key_digest[48];  // SHA-384 digest of the Author public key that certified the ID key, if provided in SNP_LAUNCH_FINISH. Zeros if author_key_en is 1 (sounds backwards to me).
    uint8_t     report_id[32];      // Report ID of this guest.
    uint8_t     report_id_ma[32];   // Report ID of this guest's mmigration agent.
    uint64_t    reported_tcb;       // Reported TCB version used to derive the VCEK that signed this report
    uint8_t     reserved2[24];      // reserved
    uint8_t     chip_id[64];        // Identifier unique to the chip
    uint8_t     committed_svn[8];   // The current commited SVN of the firware (version 2 report feature)
    uint8_t     committed_version[8];   // The current commited version of the firware
    uint8_t     launch_svn[8];      // The SVN that this guest was launched or migrated at
    uint8_t     reserved3[168];     // reserved
    uint8_t     signature[512];     // Signature of this attestation report. See table 23.
} snp_attestation_report;

/* from SEV-SNP Firmware ABI Specification Table 22 */

typedef struct {
    uint32_t status;
    uint32_t report_size;
    uint8_t reserved[24];    
    snp_attestation_report report;
    uint8_t padding[64]; // padding to the size of SEV_SNP_REPORT_RSP_BUF_SZ (i.e., 1280 bytes)
} msg_response_resp;

void printBytes(const char *desc, const uint8_t *data, size_t len, bool swap)
{
    printf("  %s: ", desc);
    int padding = 20 - strlen(desc);
    if (padding < 0)
        padding = 0;
    for (int count = 0; count < padding; count++)
        putchar(' ');

    for (size_t pos = 0; pos < len; pos++) {
        printf("%02x", data[swap ? len - pos - 1 : pos]);
        if (pos % 32 == 31)
            printf("\n                        ");
        else if (pos % 16 == 15)
            putchar(' ');
    }
    printf("\n");
}

void printReport(const snp_attestation_report *r)
{
    printf("SNP attestation report:\n");
    PRINT_VAL(r, version);              // print the version as an actual number
    PRINT_VAL(r, guest_svn);
    PRINT_VAL(r, policy);
    PRINT_VAL(r, family_id);
    PRINT_VAL(r, image_id);
    PRINT_VAL(r, vmpl);
    PRINT_VAL(r, signature_algo);
    PRINT_BYTES(r, platform_version);   // dump the platform version as hex bytes
    PRINT_BYTES(r, platform_info);
    PRINT_VAL(r, author_key_en);
    PRINT_VAL(r, reserved1);
    PRINT_BYTES(r, report_data);
    PRINT_BYTES(r, measurement);
    PRINT_BYTES(r, host_data);
    PRINT_BYTES(r, id_key_digest);
    PRINT_BYTES(r, author_key_digest);
    PRINT_BYTES(r, report_id);
    PRINT_BYTES(r, report_id_ma);
    PRINT_VAL(r, reported_tcb);
    PRINT_BYTES(r, reserved2);
    PRINT_BYTES(r, chip_id);
    PRINT_BYTES(r, committed_svn);
    PRINT_BYTES(r, committed_version);
    PRINT_BYTES(r, launch_svn);
    PRINT_BYTES(r, reserved3);
    PRINT_BYTES(r, signature);
}

uint8_t* decodeHexString(char *hexstring)
{   
    size_t len = strlen(hexstring);
    uint8_t *byte_array = (uint8_t*) malloc(strlen(hexstring)*sizeof(uint8_t));
    
    for (size_t i = 0; i < len; i+=2) {        
        sscanf(hexstring, "%2hhx", &byte_array[i/2]);
        hexstring += 2;        
    }

    return byte_array;
}


int main(int argc, char** argv)
{
    bool verbose = false;
    msg_report_req msg_report_in;    
    msg_response_resp msg_report_out;

    int fd;
    int i;
    int rc;
    
    struct sev_snp_guest_request payload = {
        .req_msg_type = SNP_MSG_REPORT_REQ,
        .rsp_msg_type = SNP_MSG_REPORT_RSP,
        .msg_version = 1,        
        .request_len = sizeof(msg_report_in),
        .request_uaddr = (uint64_t) (void*) &msg_report_in,
        .response_len = sizeof(msg_report_out),
        .response_uaddr = (uint64_t) (void*) &msg_report_out,
        .error = 0
    };
    
    memset((void*) &msg_report_in, 0, sizeof(msg_report_in));        
    memset((void*) &msg_report_out, 0, sizeof(msg_report_out));
    // default to zeros
    memset(msg_report_in.report_data, 0x00, sizeof(msg_report_in.report_data));
    if (argc > 1 && !strcmp(argv[1], "-v")) {
        verbose = true;
        argc--;
        argv++;
    }
    // todo: sha-512 key configuration structure to be used as report_data
    if (argc >1) {
        char *hexstring = argv[1];
        size_t to_copy = (strlen(hexstring)+1)/2;
        if (to_copy < sizeof(msg_report_in.report_data))
            to_copy = sizeof(msg_report_in.report_data);
        
        for (size_t i = 0; i < to_copy; i++) {        
            sscanf(hexstring, "%2hhx", &msg_report_in.report_data[i]);
            hexstring += 2;        
        }
    }

    fd = open("/dev/sev", O_RDWR | O_CLOEXEC);

    if (fd < 0) {
       printf("Failed to open /dev/sev\n");
       exit(-1);
    }

    rc = ioctl(fd, SEV_SNP_GUEST_MSG_REPORT, &payload);

    if (rc < 0) {
        printf("Failed to issue ioctl SEV_SNP_GUEST_MSG_REPORT\n");        
        exit(-1);    
    }

    if (verbose) {
        printf("Response header:\n");

        uint8_t *hdr = (uint8_t*) &msg_report_out;
        
        for (i = 0; i < 32; i++) {
           printf("%02x", hdr[i]);
           if (i % 16 == 15)
               printf("\n");
           else
               printf(" ");
       }
       printReport(&msg_report_out.report);
    } else {
        uint8_t *report_data = (uint8_t*)&msg_report_out.report;    // pointer to te memory block
        for (size_t i = 0; i < sizeof(snp_attestation_report); i++) {
            int byte = report_data[i];
            printf("%02x", byte);
        }
    }
    
    exit(0);
}
