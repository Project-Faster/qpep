#include <assert.h>

typedef struct {
        int i;
        char s[];
} t;

t x = {1, "foo"};
t y = {2, "barbaz"};

static_assert(sizeof(t) == sizeof(int), "t size incorrect");
static_assert(sizeof(x) == sizeof(int), "x size incorrect");
static_assert(sizeof(y) == sizeof(int), "y size incorrect");

int main() {}
