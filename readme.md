# forwardme

This is a Telegram message forwarding bot project that allows you to create and manage multiple Telegram bots and forward user messages to specified administrators, while also supporting user banning and appeal features.

## Project Structure

*   `Dockerfile`: Dockerfile for building amd64 architecture Docker images.
*   `Dockerfile.arm64`: Dockerfile for building arm64 architecture Docker images.
*   `compose.yml`: Configuration file for running the project using Docker Compose.
*   `data`: Directory for storing bot data (e.g., SQLite database).
*   `go.mod` & `go.sum`: Go module dependency files.
*   `main.go`: Source code for the Go program.
*   `.env`: File for storing environment variables (do not commit to the code repository).

## Building Images

### amd64 Platform

`docker build -f Dockerfile -t forwardme:amd64 .`

### arm64 Platform

`docker build -f Dockerfile.arm64 -t forwardme:arm64 .`

## Running the Project

### Using Docker Compose (Recommended)

1.  **Configure the .env File**

    Create a `.env` file in the project root directory and add the following environment variables:

    ```env
    MANAGER_BOT_TOKEN=your_manager_bot_token
    # Other environment variables (optional)
    ```

    Replace `your_manager_bot_token` with your actual Telegram manager bot token.

2.  **Run Docker Compose**

    Run the following command in the project root directory:

    `docker-compose up -d`

    This will start the containers in the background.

3.  **View Containers**

    `docker ps`

    The name of the container you created will be `my_forwardme_container`.

### Manual Run (Not Recommended)

1.  Build the image (as described above)
2.  Run the container

    `docker run -d -v ./data:/app/data -e MANAGER_BOT_TOKEN=your_manager_bot_token -p 8080:8080 janzbff/forwardme:latest`

    Please change the port mapping and other environment variables according to your needs.

## Usage

1.  **Start the Manager Bot**
    *   Use the `MANAGER_BOT_TOKEN` you specified to start your manager bot.
2.  **Create a New Bot**
    *   Send the `/newbot <bot_token>` command to the manager bot to create a new forwarding bot. Replace `<bot_token>` with the token of the bot you want to create.
3.  **Delete a Bot**
    *   Send the `/deletebot <bot_token>` command to the manager bot to delete the specified forwarding bot. Replace `<bot_token>` with the token of the bot you want to delete.
4.  **Use the Forwarding Bot**
    *   When a user sends a message to the forwarding bot, the message will be forwarded to the administrator.
    *   When the administrator replies to the message, the message will be sent to the original user.
    *   The administrator can use the `/ban <user_id>` command to ban users.
    *   The administrator can use the `/unban <user_id>` command to unban users.
    *   The administrator can use the `/getbans` command to view the currently banned users.
    *   Users will get an appeal button when they are banned, clicking it allows them to send an appeal to the administrator.
    *   Users will be permanently banned after they have appealed 3 times.
    *   The user's appeal count will be reset when they are unbanned.

## Notes

*   **Environment Variables:** The `.env` file stores sensitive information. Do not commit it to a public code repository and be sure to configure the environment variables properly.
*   **Port Mapping:** Make sure that the port mappings in `compose.yml` or `docker run` match the ports your application is listening on.
*   **Data Persistence:** Data is stored in the `data` directory, using Docker volumes for persistence.
*   **Container Naming:** The container name can be specified using the `container_name` parameter, the container name for this project is `my_forwardme_container`.

## Contribution

Contributions such as issues and pull requests are welcome.

## License

This project is under the MIT License.