typedef short int16_t;
typedef unsigned char uint8_t;
typedef unsigned long long uint64_t;

struct reftable_log_record {
	char *refname;
	uint64_t update_index; /* logical timestamp of a transactional update.
				*/

	enum {
		/* tombstone to hide deletions from earlier tables */
		REFTABLE_LOG_DELETION = 0x0,

		/* a simple update */
		REFTABLE_LOG_UPDATE = 0x1,
#define REFTABLE_NR_LOG_VALUETYPES 2
	} value_type;

	union {
		struct {
			uint8_t *new_hash;
			uint8_t *old_hash;
			char *name;
			char *email;
			uint64_t time;
			int16_t tz_offset;
			char *message;
		} update;
	} value;
	int tail;
	int tail2;
	int tail3;
};

int main() {
	uint8_t *hash1, *hash2;
	struct reftable_log_record r1[] = {
		{
			.refname = "a",
			.update_index = 2,
			.value_type = REFTABLE_LOG_UPDATE,
			.value.update = {
				.old_hash = hash2,
				/* deletion */
				.name = "jane doe",
				.email = "jane@invalid",
				.message = "message2",
			},
			.tail2 = 42,
			43,
		},
	};
}
