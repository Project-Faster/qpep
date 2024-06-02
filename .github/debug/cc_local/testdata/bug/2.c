#define H5FL_BLK_NAME(t) H5_##t##_blk_free_list
#define H5FL_BLK_AVAIL(t, size) H5FL_blk_free_block_avail(&(H5FL_BLK_NAME(t)), size)

int H5_zero_fill_blk_free_list;

void H5FL_blk_free_block_avail();

int main() {
                H5FL_BLK_AVAIL(
                    zero_fill,
                    42);
}
