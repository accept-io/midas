# MIDAS Database Setup

Simple database setup for the MIDAS authority governance engine.

## Quick Start

```bash
# Create the database (interactive)
./setup-db.sh

# Or specify the database name directly
./setup-db.sh midas
```

That's it! The script will create the database and apply the complete schema.

## Files

- **`schema.sql`** - Complete MIDAS database schema (single file)
- **`setup-db.sh`** - Setup script that creates and initializes the database

## What Gets Created

The script creates these tables:

1. **decision_surfaces** - Decision domains with versioning
2. **authority_profiles** - Authority configurations per surface
3. **agents** - Autonomous actors in the system
4. **agent_authorizations** - Grants linking agents to profiles
5. **operational_envelopes** - Five-section envelope model for decisions
6. **audit_events** - Immutable audit log with hash chain integrity

## Configuration

Use standard PostgreSQL environment variables:

```bash
export PGHOST=localhost      # default: localhost
export PGPORT=5432          # default: 5432
export PGUSER=postgres      # default: postgres
export PGPASSWORD=secret    # your password
```

Or pass them inline:

```bash
PGHOST=db.example.com PGUSER=midas ./setup-db.sh midas
```

## Examples

```bash
# Create database named 'midas' on localhost
./setup-db.sh midas

# Create database on remote host
PGHOST=db.example.com ./setup-db.sh midas_prod

# Show help
./setup-db.sh --help
```

## Migration from Old Setup

If you're migrating from the old multi-file migration structure:

1. **Back up your data** if you have an existing database
2. Run `./setup-db.sh` to create a fresh database
3. Restore your data from backup if needed

The new `schema.sql` represents the **final state** of all previous migrations combined into a single, clean schema definition.

## Development

To update the schema:

1. Edit `schema.sql` directly
2. Run `./setup-db.sh` to test on a fresh database
3. For existing databases, create a migration SQL file with the changes

## Troubleshooting

**"Database already exists"**
- The script will prompt you to drop and recreate
- Or manually: `dropdb midas && ./setup-db.sh midas`

**"Permission denied"**
- Make sure the script is executable: `chmod +x setup-db.sh`

**Connection errors**
- Check PostgreSQL is running: `psql -l`
- Verify connection parameters (PGHOST, PGPORT, PGUSER)
- Check authentication in `pg_hba.conf`

**Wrong table count**
- Should create exactly 6 tables
- If count is wrong, check the schema.sql file for errors
- Look at PostgreSQL logs for detailed error messages

## Testing

MIDAS has comprehensive test coverage across unit tests and PostgreSQL integration tests.

### Quick Start

```bash
# Run all tests (automatically starts database)
./test.sh

# Or use Make
make test
```

### Test Categories

- **Unit Tests**: Fast in-memory tests, no dependencies required
- **Integration Tests**: Validate against real PostgreSQL database

See [TESTING.md](TESTING.md) for detailed testing guide.

### Common Commands

```bash
./test.sh           # All tests
./test.sh unit      # Unit tests only (fast, no database)
./test.sh db        # Integration tests only
./test.sh coverage  # Generate coverage report
```

**Note:** Integration tests require Docker. If Docker is not available, these tests will be automatically skipped.
