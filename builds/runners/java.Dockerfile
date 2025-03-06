FROM openjdk:21-jdk-slim

RUN useradd -m -u 10000 runner && \
    mkdir -p /app && \
    chown -R runner:runner /app

WORKDIR /app
USER runner

CMD ["sh", "-c", "javac Main.java && java Main"]