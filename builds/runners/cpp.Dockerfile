FROM gcc:12.2.0

RUN useradd -m -u 10000 runner && \
    mkdir -p /app && \
    chown -R runner:runner /app

WORKDIR /app
USER runner

CMD ["sh", "-c", "g++ -Wall -Wextra -Werror -O2 -std=c++20 -o main main.cpp && ./main"]