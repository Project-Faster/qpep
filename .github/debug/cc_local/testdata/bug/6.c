#define join(a, b) join1(a, b)
#define join1(a, b) a##b
#define m(x) join(x, __LINE__)

void f(
		int m(x),
		int m(x)
) {}
