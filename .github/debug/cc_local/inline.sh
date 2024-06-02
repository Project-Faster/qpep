t=$(mktemp).c
cat << EOF > $t
inline void f() {}
int main() {}
EOF
cat $t
for opt in \
	-ansi \
	-fno-asm \
	-std=c11 \
	-std=c17 \
	-std=c18 \
	-std=c1x \
	-std=c2x \
	-std=c89 \
	-std=c90 \
	-std=c99 \
	-std=c9x \
	-std=gnu11 \
	-std=gnu17 \
	-std=gnu18 \
	-std=gnu1x \
	-std=gnu2x \
	-std=gnu89 \
	-std=gnu90 \
	-std=gnu99 \
	-std=gnu9x \
	-std=iso9899:1990 \
	-std=iso9899:199409 \
	-std=iso9899:1999 \
	-std=iso9899:199x \
	-std=iso9899:2011 \
	-std=iso9899:2017 \
	-std=iso9899:2018 \
;
do echo opt is $opt ; gcc -c -w $opt -o /dev/null $t
done
unlink $t
