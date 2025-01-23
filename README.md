# Crudder-Go Project

[![Build Status](https://github.com/mmilk23/crudder-go/actions/workflows/go.yml/badge.svg)](https://github.com/mmilk23/crudder-go/actions)
[![codecov](https://codecov.io/gh/mmilk23/crudder-go/branch/main/graph/badge.svg)](https://codecov.io/gh/mmilk23/crudder-go)
[![Coverage Status](https://coveralls.io/repos/github/mmilk23/crudder-go/badge.svg)](https://coveralls.io/github/mmilk23/crudder-go)
![Dependabot](https://img.shields.io/badge/Dependabot-enabled-brightgreen)
[![Last Updated](https://img.shields.io/github/last-commit/mmilk23/crudder-go.svg)](https://github.com/mmilk23/crudder-go/commits/main)



## Overview
**Crudder-Go** is a Go-based application designed to serve as an API for managing database operations, providing a CRUD (Create, Read, Update, Delete) interface for interacting with MySQL. This project leverages modern technologies, including Docker for container orchestration and Swagger for API documentation, making it easy to deploy and integrate.

With a focus on security and scalability, Crudder-Go includes robust session management, making it ideal for development environments, educational purposes, or small projects that need a reliable backend system for handling user data.

## Features
- **Authentication and Authorization**: Secure login using session tokens, with endpoints for login and logout operations.
- **User Management**: APIs to create users, assign roles, and manage user-related data.
- **Table Management**: Retrieve table structures, list all tables, and interact with database schemas dynamically.
- **Docker Integration**: Full Docker support, including a Docker Compose setup that brings up both the Go application and a MySQL server effortlessly.
- **Swagger Documentation**: Easy-to-navigate API documentation that allows you to test and understand all endpoints.
- **NEW in v2**: a visual tool for crudder go! just run and point your browser to http://localhost:9091/login



## Technologies Used
- **Go (Golang)**: Main language used for developing the API backend.
- **MySQL**: Database used for storing user data, roles, and other dynamic information.
- **Docker & Docker Compose**: For containerizing the Go application and the MySQL database, allowing seamless setup and scalability.
- **Swagger**: API documentation and testing interface, simplifying the interaction with endpoints. Also, docs folder contains test scripts for [Bruno tool](https://github.com/usebruno/bruno).


## Getting Started
Follow these steps to get started with Crudder-Go on your local environment:

### Prerequisites
- **Docker**: Make sure Docker and Docker Compose are installed on your system.
- **Go**: Optional, if you wish to run or modify the application code locally without Docker.

### Installation
1. **Clone the Repository**:
   ```sh
   git clone https://github.com/yourusername/crudder-go.git
   cd crudder-go
   ```

2. **Environment Configuration**:
   Ensure you have the required configuration files:
   - **`.env`**: Holds the environment variables for database credentials (if required).
   - **`init.sql`**: Initial script to create the database schema and seed initial data.

3. **Running the Project**:
   There are two ways to run the project: locally or via Docker:

   - **Running Locally** (requires Go to be installed):
     ```sh
     go mod tidy
     go run main.go
     ```

   - **Running via Docker Compose**:
     ```sh
     docker-compose up --build
     ```
   This will create both the MySQL and the Go application containers. The MySQL container will also execute the `init.sql` script to set up initial database exemple.

4. **Access the API Documentation**:
   After the containers are running, the Swagger API documentation is available at:
   [http://localhost:8080/swagger/index.html](http://localhost:8080/swagger/index.html)

### Usage
- **Authentication**: Use the `/login` endpoint to create a session, providing `username`, `password`, and `dbname` as parameters.
- **User Management**: Create users and assign roles with the `/users` and `/roles` endpoints.
- **Database Interaction**: Use `/table-structure` and `/tables` to explore and understand the structure of the database.

## Example Request
Here is an example of how to authenticate a user using cURL:

```sh
curl -X POST http://localhost:8080/login \
     -d "username=testuser" \
     -d "password=testpassword" \
     -d "dbname=testdb"
```

Upon successful login, a session token is returned in the form of a cookie that can be used for subsequent requests.

## Customization
- **Database Schema**: Modify `init.sql` to change or add database tables as required by your project.
- **Configuration**: Update environment variables in the `.env` file to change database host, user, or other configurations.
- **Swagger Annotations**: Swagger annotations are integrated into the code, ensuring the documentation is always up-to-date with the latest changes.

## Development and Testing
- **Unit Tests**: Unit tests are included for core functionality such as authentication, CRUD handlers, and session management. Run tests with:
  ```sh
  go test ./...
  ```
- **Docker Testing**: The application can be fully tested within Docker to ensure all components are working together seamlessly.

## Troubleshooting
- **MySQL Access Issues**: Ensure the MySQL container is accessible on port `3306`. If you're having trouble, check that no other service is using the port.
- **Database Initialization Issues**: If the `init.sql` script fails, verify the syntax and adjust any permissions or configurations to match your environment.

## Contributing
Contributions are welcome! Feel free to submit a pull request or open an issue for discussion.

## License
This project is licensed under the MIT License.

## Notice

This project was developed as part of a system architecture playground and is not recommended for production environments without in-depth review.

If you found this project helpful, please consider giving it a star ⭐️.

## Contact
For more information, visit the [GitHub repository](https://github.com/mmilk23/crudder-go).

